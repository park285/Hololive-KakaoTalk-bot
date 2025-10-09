package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/api/youtube/v3"
)

type YouTubeStatsService struct {
	oauth  *YouTubeOAuthService
	cache  *CacheService
	logger *zap.Logger
}

type ChannelStatistics struct {
	ChannelID        string
	SubscriberCount  uint64
	SubscriberChange int64
	VideoCount       uint64
	ViewCount        uint64
}

func NewYouTubeStatsService(oauth *YouTubeOAuthService, cache *CacheService, logger *zap.Logger) *YouTubeStatsService {
	return &YouTubeStatsService{
		oauth:  oauth,
		cache:  cache,
		logger: logger,
	}
}

func (ys *YouTubeStatsService) GetChannelStatisticsBatch(ctx context.Context, channelIDs []string) ([]*ChannelStatistics, error) {
	if ys.oauth == nil || !ys.oauth.IsAuthorized() {
		return nil, fmt.Errorf("YouTube OAuth not authorized")
	}

	service := ys.oauth.GetService()
	if service == nil {
		return nil, fmt.Errorf("YouTube service not available")
	}

	var stats []*ChannelStatistics

	const maxPerRequest = 50
	for i := 0; i < len(channelIDs); i += maxPerRequest {
		end := i + maxPerRequest
		if end > len(channelIDs) {
			end = len(channelIDs)
		}

		batch := channelIDs[i:end]
		call := service.Channels.List([]string{"statistics"}).Id(batch...)

		resp, err := call.Context(ctx).Do()
		if err != nil {
			ys.logger.Error("Failed to get channel statistics",
				zap.Int("batch_size", len(batch)),
				zap.Error(err))
			continue
		}

		for _, item := range resp.Items {
			stats = append(stats, &ChannelStatistics{
				ChannelID:       item.Id,
				SubscriberCount: item.Statistics.SubscriberCount,
				VideoCount:      item.Statistics.VideoCount,
				ViewCount:       item.Statistics.ViewCount,
			})
		}
	}

	return stats, nil
}

func (ys *YouTubeStatsService) GetRecentVideos(ctx context.Context, channelID string, maxResults int64) ([]*youtube.SearchResult, error) {
	if ys.oauth == nil || !ys.oauth.IsAuthorized() {
		return nil, fmt.Errorf("YouTube OAuth not authorized")
	}

	service := ys.oauth.GetService()
	if service == nil {
		return nil, fmt.Errorf("YouTube service not available")
	}

	call := service.Search.List([]string{"id", "snippet"}).
		ChannelId(channelID).
		Type("video").
		Order("date").
		MaxResults(maxResults)

	resp, err := call.Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("failed to search videos: %w", err)
	}

	return resp.Items, nil
}
