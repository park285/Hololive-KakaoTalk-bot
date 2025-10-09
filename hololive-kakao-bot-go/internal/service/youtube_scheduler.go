package service

import (
	"context"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
)

// YouTubeScheduler manages periodic YouTube API tasks for quota building
type YouTubeScheduler struct {
	youtube       *YouTubeService
	cache         *CacheService
	membersData   *domain.MembersData
	logger        *zap.Logger
	ticker        *time.Ticker
	stopCh        chan struct{}
	currentBatch  int
	batchMu       sync.Mutex
}

const (
	// Schedule intervals (하루 2번: 오전/오후)
	schedulerInterval = 12 * time.Hour

	// Batch configuration for 6,000 units/day (quota 절약)
	channelsPerBatch = 30  // 30 channels × 100 units = 3,000 units per batch
	batchesPerDay    = 2   // 2 batches × 3,000 = 6,000 units
	totalDailyQuota  = 6000
)

// NewYouTubeScheduler creates a new YouTube scheduler
func NewYouTubeScheduler(youtube *YouTubeService, cache *CacheService, membersData *domain.MembersData, logger *zap.Logger) *YouTubeScheduler {
	return &YouTubeScheduler{
		youtube:      youtube,
		cache:        cache,
		membersData:  membersData,
		logger:       logger,
		currentBatch: 0,
		stopCh:       make(chan struct{}),
	}
}

// Start begins the periodic YouTube API tasks
func (ys *YouTubeScheduler) Start(ctx context.Context) {
	ys.ticker = time.NewTicker(schedulerInterval)

	ys.logger.Info("YouTube quota building scheduler started",
		zap.Duration("interval", schedulerInterval),
		zap.Int("channels_per_batch", channelsPerBatch),
		zap.Int("daily_quota_target", totalDailyQuota))

	// 즉시 실행 제거 (quota 절약, 12시간 간격만 유지)
	// go ys.runBatch(ctx)

	// Run on schedule only (12시간마다)
	go func() {
		for {
			select {
			case <-ys.ticker.C:
				ys.runBatch(ctx)
			case <-ys.stopCh:
				ys.logger.Info("YouTube scheduler stopped")
				return
			case <-ctx.Done():
				ys.logger.Info("YouTube scheduler context cancelled")
				return
			}
		}
	}()
}

// Stop stops the scheduler
func (ys *YouTubeScheduler) Stop() {
	if ys.ticker != nil {
		ys.ticker.Stop()
	}
	close(ys.stopCh)
}

// runBatch executes one batch of YouTube API calls
func (ys *YouTubeScheduler) runBatch(ctx context.Context) {
	ys.batchMu.Lock()
	batchNum := ys.currentBatch
	ys.currentBatch = (ys.currentBatch + 1) % batchesPerDay
	ys.batchMu.Unlock()

	ys.logger.Info("Running YouTube quota building batch",
		zap.Int("batch", batchNum),
		zap.Int("total_batches", batchesPerDay))

	// Task 1: Track subscriber changes for ALL members (192 units)
	go ys.trackAllSubscribers(ctx)

	// Task 2: Fetch recent videos for rotating subset (3,000 units)
	go ys.fetchRecentVideosRotation(ctx, batchNum)
}

// trackAllSubscribers tracks subscriber counts for all 64 members (64 units)
func (ys *YouTubeScheduler) trackAllSubscribers(ctx context.Context) {
	channelIDs := make([]string, 0, len(ys.membersData.Members))
	for _, member := range ys.membersData.Members {
		channelIDs = append(channelIDs, member.ChannelID)
	}

	ys.logger.Info("Tracking all member subscribers",
		zap.Int("channels", len(channelIDs)),
		zap.Int("quota_cost", len(channelIDs)))

	stats, err := ys.youtube.GetChannelStatistics(ctx, channelIDs)
	if err != nil {
		ys.logger.Error("Failed to track subscribers", zap.Error(err))
		return
	}

	// Save stats and calculate changes
	changesDetected := 0
	for channelID, currentStats := range stats {
		// Try to get previous stats
		var prevStats *ChannelStats
		cacheKey := "youtube:stats:prev:" + channelID
		if err := ys.cache.Get(ctx, cacheKey, &prevStats); err == nil && prevStats != nil {
			// Calculate change
			subChange := int64(currentStats.SubscriberCount) - int64(prevStats.SubscriberCount)
			vidChange := int64(currentStats.VideoCount) - int64(prevStats.VideoCount)

			if subChange != 0 || vidChange != 0 {
				ys.logger.Info("Channel stats changed",
					zap.String("channel", channelID),
					zap.Int64("sub_change", subChange),
					zap.Int64("vid_change", vidChange))
				changesDetected++
			}
		}

		// Save current stats as previous for next check
		ys.cache.Set(ctx, cacheKey, currentStats, 25*time.Hour) // 25h to handle schedule drift
	}

	ys.logger.Info("Subscriber tracking completed",
		zap.Int("tracked", len(stats)),
		zap.Int("changes", changesDetected))
}

// fetchRecentVideosRotation fetches recent videos for a rotating subset (3,000 units)
func (ys *YouTubeScheduler) fetchRecentVideosRotation(ctx context.Context, batchNum int) {
	// Get rotating batch of 30 channels
	channels := ys.getRotatingBatch(batchNum, channelsPerBatch)

	ys.logger.Info("Fetching recent videos for batch",
		zap.Int("batch", batchNum),
		zap.Int("channels", len(channels)),
		zap.Int("quota_cost", len(channels)*100))

	successCount := 0
	errorCount := 0

	for _, channelID := range channels {
		videos, err := ys.youtube.GetRecentVideos(ctx, channelID, 10)
		if err != nil {
			ys.logger.Warn("Failed to fetch recent videos",
				zap.String("channel", channelID),
				zap.Error(err))
			errorCount++
			continue
		}

		// Cache recent videos
		cacheKey := "youtube:recent_videos:" + channelID
		ys.cache.Set(ctx, cacheKey, videos, 24*time.Hour)

		ys.logger.Debug("Recent videos fetched",
			zap.String("channel", channelID),
			zap.Int("videos", len(videos)))

		successCount++
	}

	ys.logger.Info("Recent videos batch completed",
		zap.Int("batch", batchNum),
		zap.Int("success", successCount),
		zap.Int("errors", errorCount))
}

// getRotatingBatch returns a rotating subset of channel IDs
func (ys *YouTubeScheduler) getRotatingBatch(batchNum int, size int) []string {
	allChannels := make([]string, 0, len(ys.membersData.Members))
	for _, member := range ys.membersData.Members {
		allChannels = append(allChannels, member.ChannelID)
	}

	total := len(allChannels)
	start := (batchNum * size) % total
	end := start + size

	if end <= total {
		return allChannels[start:end]
	}

	// Wrap around
	batch := make([]string, 0, size)
	batch = append(batch, allChannels[start:]...)
	batch = append(batch, allChannels[0:end-total]...)

	return batch
}
