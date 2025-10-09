package service

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
)

type YouTubeScheduler struct {
	youtube      *YouTubeService
	cache        *CacheService
	statsRepo    *YouTubeStatsRepository
	membersData  domain.MemberDataProvider
	logger       *zap.Logger
	ticker       *time.Ticker
	stopCh       chan struct{}
	currentBatch int
	batchMu      sync.Mutex
}

const (
	schedulerInterval = 12 * time.Hour

	channelsPerBatch = 30 // 30 channels × 100 units = 3,000 units per batch
	batchesPerDay    = 2  // 2 batches × 3,000 = 6,000 units
	totalDailyQuota  = 6000
)

func NewYouTubeScheduler(youtube *YouTubeService, cache *CacheService, statsRepo *YouTubeStatsRepository, membersData domain.MemberDataProvider, logger *zap.Logger) *YouTubeScheduler {
	return &YouTubeScheduler{
		youtube:      youtube,
		cache:        cache,
		statsRepo:    statsRepo,
		membersData:  membersData,
		logger:       logger,
		currentBatch: 0,
		stopCh:       make(chan struct{}),
	}
}

func (ys *YouTubeScheduler) Start(ctx context.Context) {
	ys.ticker = time.NewTicker(schedulerInterval)

	ys.logger.Info("YouTube quota building scheduler started",
		zap.Duration("interval", schedulerInterval),
		zap.Int("channels_per_batch", channelsPerBatch),
		zap.Int("daily_quota_target", totalDailyQuota))

	// 즉시 실행 제거 (quota 절약, 12시간 간격만 유지)
	// go ys.runBatch(ctx)

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

func (ys *YouTubeScheduler) Stop() {
	if ys.ticker != nil {
		ys.ticker.Stop()
	}
	close(ys.stopCh)
}

func (ys *YouTubeScheduler) runBatch(ctx context.Context) {
	ys.batchMu.Lock()
	batchNum := ys.currentBatch
	ys.currentBatch = (ys.currentBatch + 1) % batchesPerDay
	ys.batchMu.Unlock()

	ys.logger.Info("Running YouTube quota building batch",
		zap.Int("batch", batchNum),
		zap.Int("total_batches", batchesPerDay))

	go ys.trackAllSubscribers(ctx)

	go ys.fetchRecentVideosRotation(ctx, batchNum)
}

func (ys *YouTubeScheduler) trackAllSubscribers(ctx context.Context) {
	channelIDs := make([]string, 0, len(ys.membersData.GetAllMembers()))
	channelToMember := make(map[string]*domain.Member)

	for _, member := range ys.membersData.GetAllMembers() {
		channelIDs = append(channelIDs, member.ChannelID)
		channelToMember[member.ChannelID] = member
	}

	ys.logger.Info("Tracking all member subscribers",
		zap.Int("channels", len(channelIDs)),
		zap.Int("quota_cost", len(channelIDs)))

	stats, err := ys.youtube.GetChannelStatistics(ctx, channelIDs)
	if err != nil {
		ys.logger.Error("Failed to track subscribers", zap.Error(err))
		return
	}

	now := time.Now()
	changesDetected := 0
	milestonesAchieved := 0

	for channelID, currentStats := range stats {
		member := channelToMember[channelID]
		if member == nil {
			continue
		}

		prevStats, err := ys.statsRepo.GetLatestStats(ctx, channelID)
		if err != nil {
			ys.logger.Warn("Failed to get previous stats",
				zap.String("channel", channelID),
				zap.Error(err))
		}

		timestampedStats := &domain.TimestampedStats{
			ChannelID:       channelID,
			MemberName:      member.Name,
			SubscriberCount: uint64(currentStats.SubscriberCount),
			VideoCount:      uint64(currentStats.VideoCount),
			ViewCount:       uint64(currentStats.ViewCount),
			Timestamp:       now,
		}

		if err := ys.statsRepo.SaveStats(ctx, timestampedStats); err != nil {
			ys.logger.Error("Failed to save stats",
				zap.String("channel", channelID),
				zap.Error(err))
			continue
		}

		if prevStats != nil {
			subChange := int64(currentStats.SubscriberCount) - int64(prevStats.SubscriberCount)
			vidChange := int64(currentStats.VideoCount) - int64(prevStats.VideoCount)
			viewChange := int64(currentStats.ViewCount) - int64(prevStats.ViewCount)

			if subChange != 0 || vidChange != 0 {
				change := &domain.StatsChange{
					ChannelID:        channelID,
					MemberName:       member.Name,
					SubscriberChange: subChange,
					VideoChange:      vidChange,
					ViewChange:       viewChange,
					PreviousStats:    prevStats,
					CurrentStats:     timestampedStats,
					DetectedAt:       now,
				}

				if err := ys.statsRepo.RecordChange(ctx, change); err != nil {
					ys.logger.Error("Failed to record change",
						zap.String("member", member.Name),
						zap.Error(err))
				} else {
					changesDetected++
				}

				milestones := ys.checkMilestones(prevStats.SubscriberCount, uint64(currentStats.SubscriberCount))
				for _, milestone := range milestones {
					milestoneRecord := &domain.Milestone{
						ChannelID:  channelID,
						MemberName: member.Name,
						Type:       domain.MilestoneSubscribers,
						Value:      milestone,
						AchievedAt: now,
						Notified:   false,
					}

					if err := ys.statsRepo.SaveMilestone(ctx, milestoneRecord); err != nil {
						ys.logger.Error("Failed to save milestone",
							zap.String("member", member.Name),
							zap.Uint64("value", milestone),
							zap.Error(err))
					} else {
						milestonesAchieved++
						ys.logger.Info("🎉 Milestone achieved!",
							zap.String("member", member.Name),
							zap.Uint64("subscribers", milestone),
						)
					}
				}
			}
		}
	}

	ys.logger.Info("Subscriber tracking completed",
		zap.Int("tracked", len(stats)),
		zap.Int("changes", changesDetected),
		zap.Int("milestones", milestonesAchieved))
}

func (ys *YouTubeScheduler) fetchRecentVideosRotation(ctx context.Context, batchNum int) {
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

func (ys *YouTubeScheduler) getRotatingBatch(batchNum int, size int) []string {
	allChannels := make([]string, 0, len(ys.membersData.GetAllMembers()))
	for _, member := range ys.membersData.GetAllMembers() {
		allChannels = append(allChannels, member.ChannelID)
	}

	total := len(allChannels)
	start := (batchNum * size) % total
	end := start + size

	if end <= total {
		return allChannels[start:end]
	}

	batch := make([]string, 0, size)
	batch = append(batch, allChannels[start:]...)
	batch = append(batch, allChannels[0:end-total]...)

	return batch
}


func (ys *YouTubeScheduler) checkMilestones(prevCount, currentCount uint64) []uint64 {
	milestones := []uint64{
		100000,    // 10만
		250000,    // 25만
		500000,    // 50만
		750000,    // 75만
		1000000,   // 100만
		1500000,   // 150만
		2000000,   // 200만
		2500000,   // 250만
		3000000,   // 300만
		4000000,   // 400만
		5000000,   // 500만
		10000000,  // 1000만
	}

	var achieved []uint64
	for _, milestone := range milestones {
		if prevCount < milestone && currentCount >= milestone {
			achieved = append(achieved, milestone)
		}
	}

	return achieved
}


func (ys *YouTubeScheduler) SendMilestoneNotifications(ctx context.Context, sendMessage func(room, message string) error, rooms []string) error {
	changes, err := ys.statsRepo.GetUnnotifiedChanges(ctx, 50)
	if err != nil {
		return fmt.Errorf("failed to get unnotified changes: %w", err)
	}

	if len(changes) == 0 {
		return nil
	}

	ys.logger.Info("Processing stats changes for notifications",
		zap.Int("changes", len(changes)))

	sentCount := 0
	for _, change := range changes {
		if !ys.isSignificantChange(change) {
			if err := ys.statsRepo.MarkChangeNotified(ctx, change.ChannelID, change.DetectedAt); err != nil {
				ys.logger.Warn("Failed to mark change notified",
					zap.String("channel", change.ChannelID),
					zap.Error(err))
			}
			continue
		}

		message := ys.formatChangeMessage(change)
		if message == "" {
			continue
		}

		for _, room := range rooms {
			if err := sendMessage(room, message); err != nil {
				ys.logger.Error("Failed to send milestone notification",
					zap.String("room", room),
					zap.String("member", change.MemberName),
					zap.Error(err))
				continue
			}
		}

		if err := ys.statsRepo.MarkChangeNotified(ctx, change.ChannelID, change.DetectedAt); err != nil {
			ys.logger.Warn("Failed to mark change notified",
				zap.String("channel", change.ChannelID),
				zap.Error(err))
		} else {
			sentCount++
		}
	}

	if sentCount > 0 {
		ys.logger.Info("Milestone notifications sent",
			zap.Int("sent", sentCount))
	}

	return nil
}

func (ys *YouTubeScheduler) isSignificantChange(change *domain.StatsChange) bool {
	if change.SubscriberChange >= 10000 {
		return true
	}

	if change.PreviousStats != nil && change.CurrentStats != nil {
		milestones := ys.checkMilestones(change.PreviousStats.SubscriberCount, change.CurrentStats.SubscriberCount)
		if len(milestones) > 0 {
			return true
		}
	}

	return false
}

func (ys *YouTubeScheduler) formatChangeMessage(change *domain.StatsChange) string {
	if change.PreviousStats == nil || change.CurrentStats == nil {
		return ""
	}

	milestones := ys.checkMilestones(change.PreviousStats.SubscriberCount, change.CurrentStats.SubscriberCount)
	if len(milestones) > 0 {
		milestone := milestones[0] // Take first milestone
		return fmt.Sprintf("🎉 %s님이 구독자 %s명을 달성했습니다!\n축하합니다! 🎊",
			change.MemberName,
			formatNumber(milestone))
	}

	if change.SubscriberChange >= 10000 {
		return fmt.Sprintf("📈 %s님의 구독자가 %s명 증가했습니다!\n현재 구독자: %s명",
			change.MemberName,
			formatNumber(uint64(change.SubscriberChange)),
			formatNumber(change.CurrentStats.SubscriberCount))
	}

	return ""
}

func formatNumber(n uint64) string {
	if n >= 10000 {
		man := n / 10000
		remainder := n % 10000
		if remainder == 0 {
			return fmt.Sprintf("%d만", man)
		}
		return fmt.Sprintf("%d만 %d", man, remainder)
	}
	return fmt.Sprintf("%d", n)
}
