package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	"google.golang.org/api/youtube/v3"
)

// YouTubeService provides backup access to YouTube Data API v3
// WARNING: This is a BACKUP service due to strict quota limits
type YouTubeService struct {
	service    *youtube.Service
	cache      *CacheService
	logger     *zap.Logger
	quotaUsed  int
	quotaMu    sync.Mutex
	quotaReset time.Time
}

const (
	// YouTube Data API quota limits (per day)
	dailyQuotaLimit   = 10000
	searchQuotaCost   = 100 // search.list cost
	channelsQuotaCost = 1   // channels.list cost

	// Conservative limits for backup usage
	maxChannelsPerCall = 20              // Max 2000 units per call
	quotaSafetyMargin  = 2000            // Reserve 2000 units
	cacheExpiration    = 2 * time.Hour   // Cache for 2 hours
)

// NewYouTubeService creates a YouTube API backup service with quota management
func NewYouTubeService(apiKey string, cache *CacheService, logger *zap.Logger) (*YouTubeService, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("YouTube API key is required")
	}

	ctx := context.Background()
	service, err := youtube.NewService(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create YouTube service: %w", err)
	}

	ys := &YouTubeService{
		service:    service,
		cache:      cache,
		logger:     logger,
		quotaUsed:  0,
		quotaReset: getNextQuotaReset(),
	}

	logger.Info("YouTube backup service initialized",
		zap.Time("quotaReset", ys.quotaReset))

	return ys, nil
}

// getNextQuotaReset calculates next quota reset time (midnight Pacific Time)
func getNextQuotaReset() time.Time {
	pt, _ := time.LoadLocation("America/Los_Angeles")
	now := time.Now().In(pt)
	next := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, pt)
	return next
}

// checkQuota verifies if we have enough quota for the operation
func (ys *YouTubeService) checkQuota(cost int) error {
	ys.quotaMu.Lock()
	defer ys.quotaMu.Unlock()

	// Auto-reset quota if new day
	now := time.Now()
	if now.After(ys.quotaReset) {
		ys.quotaUsed = 0
		ys.quotaReset = getNextQuotaReset()
		ys.logger.Info("YouTube API quota auto-reset",
			zap.Time("nextReset", ys.quotaReset))
	}

	// Check if we have enough quota (with safety margin)
	if ys.quotaUsed+cost > (dailyQuotaLimit - quotaSafetyMargin) {
		return &QuotaExceededError{
			Used:      ys.quotaUsed,
			Limit:     dailyQuotaLimit,
			Requested: cost,
			ResetTime: ys.quotaReset,
		}
	}

	return nil
}

// consumeQuota marks quota as used after successful API call
func (ys *YouTubeService) consumeQuota(cost int) {
	ys.quotaMu.Lock()
	defer ys.quotaMu.Unlock()

	ys.quotaUsed += cost
	remaining := dailyQuotaLimit - ys.quotaUsed

	ys.logger.Debug("YouTube API quota consumed",
		zap.Int("cost", cost),
		zap.Int("used", ys.quotaUsed),
		zap.Int("remaining", remaining),
		zap.Float64("usagePercent", float64(ys.quotaUsed)/float64(dailyQuotaLimit)*100))

	// Warn if quota usage is high
	if remaining < quotaSafetyMargin {
		ys.logger.Warn("YouTube API quota running low",
			zap.Int("remaining", remaining),
			zap.Time("resetTime", ys.quotaReset))
	}
}

// GetUpcomingStreams fetches upcoming streams for SELECTED channels only
// This is a BACKUP method - use sparingly due to quota limits
func (ys *YouTubeService) GetUpcomingStreams(ctx context.Context, channelIDs []string) ([]*domain.Stream, error) {
	// Limit channel count to conserve quota
	if len(channelIDs) > maxChannelsPerCall {
		ys.logger.Warn("Too many channels requested, limiting to max",
			zap.Int("requested", len(channelIDs)),
			zap.Int("limited", maxChannelsPerCall))
		channelIDs = channelIDs[:maxChannelsPerCall]
	}

	// Check cache first
	cacheKey := fmt.Sprintf("youtube:upcoming:%d", len(channelIDs))
	if cached, found := ys.cache.GetStreams(cacheKey); found {
		ys.logger.Debug("YouTube cache hit (backup avoided)",
			zap.Int("streams", len(cached)))
		return cached, nil
	}

	// Check quota BEFORE making API calls
	estimatedCost := len(channelIDs) * searchQuotaCost
	if err := ys.checkQuota(estimatedCost); err != nil {
		return nil, err
	}

	ys.logger.Info("Fetching from YouTube API (BACKUP MODE)",
		zap.Int("channels", len(channelIDs)),
		zap.Int("estimatedCost", estimatedCost))

	var allStreams []*domain.Stream
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(channelIDs))

	// Fetch with concurrency limit (max 3 concurrent to avoid rate limit)
	semaphore := make(chan struct{}, 3)

	actualCost := 0
	costMu := sync.Mutex{}

	for _, channelID := range channelIDs {
		wg.Add(1)
		go func(chID string) {
			defer wg.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			streams, err := ys.getChannelUpcomingStreams(ctx, chID)
			if err != nil {
				errChan <- fmt.Errorf("channel %s: %w", chID, err)
				return
			}

			mu.Lock()
			allStreams = append(allStreams, streams...)
			mu.Unlock()

			costMu.Lock()
			actualCost += searchQuotaCost
			costMu.Unlock()
		}(channelID)
	}

	wg.Wait()
	close(errChan)

	// Consume actual quota used
	ys.consumeQuota(actualCost)

	// Collect errors (if any)
	var errors []error
	for err := range errChan {
		errors = append(errors, err)
	}

	if len(errors) > 0 {
		ys.logger.Warn("Some YouTube API calls failed",
			zap.Int("failures", len(errors)),
			zap.Int("successes", len(channelIDs)-len(errors)))
		// Continue with partial results
	}

	if len(allStreams) == 0 && len(errors) > 0 {
		return nil, fmt.Errorf("all YouTube API calls failed: %d errors", len(errors))
	}

	// Cache successful results
	ys.cache.SetStreams(cacheKey, allStreams, cacheExpiration)

	ys.logger.Info("YouTube API backup completed",
		zap.Int("channels", len(channelIDs)),
		zap.Int("streams", len(allStreams)),
		zap.Int("quotaUsed", actualCost))

	return allStreams, nil
}

// getChannelUpcomingStreams fetches upcoming streams for a single channel
func (ys *YouTubeService) getChannelUpcomingStreams(ctx context.Context, channelID string) ([]*domain.Stream, error) {
	call := ys.service.Search.List([]string{"snippet"}).
		ChannelId(channelID).
		Type("video").
		EventType("upcoming").
		MaxResults(10).
		Order("date")

	response, err := call.Context(ctx).Do()
	if err != nil {
		if apiErr, ok := err.(*googleapi.Error); ok {
			if apiErr.Code == 403 {
				return nil, &QuotaExceededError{
					Used:      ys.quotaUsed,
					Limit:     dailyQuotaLimit,
					Requested: searchQuotaCost,
					ResetTime: ys.quotaReset,
				}
			}
		}
		return nil, fmt.Errorf("YouTube API error: %w", err)
	}

	streams := make([]*domain.Stream, 0, len(response.Items))
	for _, item := range response.Items {
		if item.Id == nil || item.Id.VideoId == "" {
			continue
		}

		stream := &domain.Stream{
			ID:        item.Id.VideoId,
			Title:     item.Snippet.Title,
			ChannelID: channelID,
			Status:    domain.StreamStatusUpcoming,
			Link:      stringPtr(fmt.Sprintf("https://www.youtube.com/watch?v=%s", item.Id.VideoId)),
			Thumbnail: extractThumbnail(item.Snippet.Thumbnails),
		}

		// Parse scheduled start time
		if item.Snippet.PublishedAt != "" {
			if startTime, err := time.Parse(time.RFC3339, item.Snippet.PublishedAt); err == nil {
				stream.StartScheduled = &startTime
			}
		}

		// Add channel info
		if item.Snippet.ChannelTitle != "" {
			stream.Channel = &domain.Channel{
				ID:   channelID,
				Name: item.Snippet.ChannelTitle,
			}
		}

		streams = append(streams, stream)
	}

	return streams, nil
}

// extractThumbnail gets the best quality thumbnail URL
func extractThumbnail(thumbnails *youtube.ThumbnailDetails) *string {
	if thumbnails == nil {
		return nil
	}

	// Try from highest to lowest quality
	if thumbnails.Maxres != nil && thumbnails.Maxres.Url != "" {
		return &thumbnails.Maxres.Url
	}
	if thumbnails.High != nil && thumbnails.High.Url != "" {
		return &thumbnails.High.Url
	}
	if thumbnails.Medium != nil && thumbnails.Medium.Url != "" {
		return &thumbnails.Medium.Url
	}
	if thumbnails.Default != nil && thumbnails.Default.Url != "" {
		return &thumbnails.Default.Url
	}

	return nil
}

// GetQuotaStatus returns current quota usage information
func (ys *YouTubeService) GetQuotaStatus() (used int, remaining int, resetTime time.Time) {
	ys.quotaMu.Lock()
	defer ys.quotaMu.Unlock()

	// Check if quota reset occurred
	if time.Now().After(ys.quotaReset) {
		return 0, dailyQuotaLimit, getNextQuotaReset()
	}

	return ys.quotaUsed, dailyQuotaLimit - ys.quotaUsed, ys.quotaReset
}

// IsQuotaAvailable checks if YouTube API can be used (has sufficient quota)
func (ys *YouTubeService) IsQuotaAvailable(channelCount int) bool {
	estimatedCost := channelCount * searchQuotaCost
	err := ys.checkQuota(estimatedCost)
	return err == nil
}

// QuotaExceededError represents a quota limit error
type QuotaExceededError struct {
	Used      int
	Limit     int
	Requested int
	ResetTime time.Time
}

func (e *QuotaExceededError) Error() string {
	return fmt.Sprintf("YouTube API quota exceeded: used %d/%d (requested %d more), resets at %s",
		e.Used, e.Limit, e.Requested, e.ResetTime.Format(time.RFC3339))
}

func stringPtr(s string) *string {
	return &s
}

// GetChannelStatistics fetches channel statistics (1 unit per channel)
func (ys *YouTubeService) GetChannelStatistics(ctx context.Context, channelIDs []string) (map[string]*ChannelStats, error) {
	if len(channelIDs) == 0 {
		return make(map[string]*ChannelStats), nil
	}

	// Check quota
	cost := len(channelIDs)
	if err := ys.checkQuota(cost); err != nil {
		return nil, err
	}

	result := make(map[string]*ChannelStats)

	// YouTube allows up to 50 channels per request
	batchSize := 50
	for i := 0; i < len(channelIDs); i += batchSize {
		end := i + batchSize
		if end > len(channelIDs) {
			end = len(channelIDs)
		}

		batch := channelIDs[i:end]

		call := ys.service.Channels.List([]string{"statistics", "snippet"}).
			Id(batch...)

		response, err := call.Context(ctx).Do()
		if err != nil {
			ys.logger.Error("Failed to fetch channel statistics",
				zap.Int("batch_size", len(batch)),
				zap.Error(err))
			continue
		}

		for _, channel := range response.Items {
			stats := &ChannelStats{
				ChannelID:       channel.Id,
				ChannelTitle:    channel.Snippet.Title,
				SubscriberCount: channel.Statistics.SubscriberCount,
				VideoCount:      channel.Statistics.VideoCount,
				ViewCount:       channel.Statistics.ViewCount,
				Timestamp:       time.Now(),
			}
			result[channel.Id] = stats
		}
	}

	// Consume quota
	ys.consumeQuota(cost)

	ys.logger.Info("Channel statistics fetched",
		zap.Int("channels", len(channelIDs)),
		zap.Int("results", len(result)),
		zap.Int("quota_used", cost))

	return result, nil
}

// GetRecentVideos fetches recent uploads for a channel (100 units)
func (ys *YouTubeService) GetRecentVideos(ctx context.Context, channelID string, maxResults int64) ([]string, error) {
	// Check quota
	if err := ys.checkQuota(searchQuotaCost); err != nil {
		return nil, err
	}

	call := ys.service.Search.List([]string{"id"}).
		ChannelId(channelID).
		Type("video").
		Order("date").
		MaxResults(maxResults)

	response, err := call.Context(ctx).Do()
	if err != nil {
		ys.logger.Error("Failed to fetch recent videos",
			zap.String("channel", channelID),
			zap.Error(err))
		return nil, fmt.Errorf("YouTube search error: %w", err)
	}

	videoIDs := make([]string, 0, len(response.Items))
	for _, item := range response.Items {
		if item.Id != nil && item.Id.VideoId != "" {
			videoIDs = append(videoIDs, item.Id.VideoId)
		}
	}

	// Consume quota
	ys.consumeQuota(searchQuotaCost)

	ys.logger.Debug("Recent videos fetched",
		zap.String("channel", channelID),
		zap.Int("count", len(videoIDs)))

	return videoIDs, nil
}

// ChannelStats represents YouTube channel statistics
type ChannelStats struct {
	ChannelID       string
	ChannelTitle    string
	SubscriberCount uint64
	VideoCount      uint64
	ViewCount       uint64
	Timestamp       time.Time
}
