package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"github.com/sourcegraph/conc/pool"
	"go.uber.org/zap"
)

// Redis key constants
const (
	AlarmKeyPrefix              = "alarm:"
	AlarmRegistryKey            = "alarm:registry"
	AlarmChannelRegistryKey     = "alarm:channel_registry"
	ChannelSubscribersKeyPrefix = "alarm:channel_subscribers:"
	MemberNameKey               = "member_names"
	NotifiedKeyPrefix           = "notified:"
	NextStreamKeyPrefix         = "alarm:next_stream:"
)

// NotifiedData represents notification cache entry
type NotifiedData struct {
	StartScheduled string `json:"start_scheduled"`
	NotifiedAt     string `json:"notified_at"`
	MinutesUntil   int    `json:"minutes_until"`
}

// AlarmService manages stream notification alarms
type AlarmService struct {
	cache         *CacheService
	holodex       *HolodexService
	logger        *zap.Logger
	targetMinutes []int
	concurrency   int
	cacheMutex    sync.RWMutex // Protects next stream cache updates (RW for concurrent reads)
}

// NewAlarmService creates a new AlarmService
func NewAlarmService(cache *CacheService, holodex *HolodexService, logger *zap.Logger, advanceMinutes []int) *AlarmService {
	// Build fallback chain
	primary := 5
	if len(advanceMinutes) > 0 {
		primary = advanceMinutes[0]
	}

	fallback1 := util.Max(1, primary-2)
	fallback2 := 1

	targetMinutes := util.Unique([]int{primary, fallback1, fallback2})

	logger.Info("Alarm fallback chain",
		zap.String("chain", fmt.Sprintf("%v분", targetMinutes)),
	)

	return &AlarmService{
		cache:         cache,
		holodex:       holodex,
		logger:        logger,
		targetMinutes: targetMinutes,
		concurrency:   15,
	}
}

// AddAlarm adds a user alarm
func (as *AlarmService) AddAlarm(ctx context.Context, roomID, userID, channelID, memberName string) (bool, error) {
	// User alarm set
	alarmKey := as.getAlarmKey(roomID, userID)
	added, err := as.cache.SAdd(ctx, alarmKey, []string{channelID})
	if err != nil {
		as.logger.Error("Failed to add alarm", zap.Error(err))
		return false, err
	}

	// Registry
	registryKey := as.getRegistryKey(roomID, userID)
	if _, err := as.cache.SAdd(ctx, AlarmRegistryKey, []string{registryKey}); err != nil {
		as.logger.Warn("Failed to add to registry", zap.Error(err))
	}

	// Channel subscribers
	channelSubsKey := as.getChannelSubscribersKey(channelID)
	if _, err := as.cache.SAdd(ctx, channelSubsKey, []string{registryKey}); err != nil {
		as.logger.Warn("Failed to add channel subscriber", zap.Error(err))
	}

	// Channel registry
	if _, err := as.cache.SAdd(ctx, AlarmChannelRegistryKey, []string{channelID}); err != nil {
		as.logger.Warn("Failed to add to channel registry", zap.Error(err))
	}

	// Cache member name
	if err := as.CacheMemberName(ctx, channelID, memberName); err != nil {
		as.logger.Warn("Failed to cache member name", zap.Error(err))
	}

	// Cache next stream info (alarm checker will update every 60s, no need for immediate update)

	as.logger.Info("Alarm added",
		zap.String("room_id", roomID),
		zap.String("user_id", userID),
		zap.String("channel_id", channelID),
		zap.String("member_name", memberName),
	)

	return added > 0, nil
}

// RemoveAlarm removes a user alarm
func (as *AlarmService) RemoveAlarm(ctx context.Context, roomID, userID, channelID string) (bool, error) {
	alarmKey := as.getAlarmKey(roomID, userID)
	removed, err := as.cache.SRem(ctx, alarmKey, []string{channelID})
	if err != nil {
		as.logger.Error("Failed to remove alarm", zap.Error(err))
		return false, err
	}

	registryKey := as.getRegistryKey(roomID, userID)
	channelSubsKey := as.getChannelSubscribersKey(channelID)

	// Remove from channel subscribers
	if _, err := as.cache.SRem(ctx, channelSubsKey, []string{registryKey}); err != nil {
		as.logger.Warn("Failed to remove from channel subscribers", zap.Error(err))
	}

	// Check if channel has remaining subscribers
	remainingSubs, err := as.cache.SMembers(ctx, channelSubsKey)
	if err == nil && len(remainingSubs) == 0 {
		as.cache.SRem(ctx, AlarmChannelRegistryKey, []string{channelID})
		as.cache.Del(ctx, channelSubsKey)
	}

	// Check if user has remaining alarms
	remainingAlarms, err := as.cache.SMembers(ctx, alarmKey)
	if err == nil && len(remainingAlarms) == 0 {
		as.cache.SRem(ctx, AlarmRegistryKey, []string{registryKey})
		as.logger.Debug("User removed from registry (no alarms left)",
			zap.String("room_id", roomID),
			zap.String("user_id", userID),
		)
	}

	as.logger.Info("Alarm removed",
		zap.String("room_id", roomID),
		zap.String("user_id", userID),
		zap.String("channel_id", channelID),
	)

	return removed > 0, nil
}

// GetUserAlarms returns user's alarm channel IDs
func (as *AlarmService) GetUserAlarms(ctx context.Context, roomID, userID string) ([]string, error) {
	alarmKey := as.getAlarmKey(roomID, userID)
	channelIDs, err := as.cache.SMembers(ctx, alarmKey)
	if err != nil {
		as.logger.Error("Failed to get user alarms", zap.Error(err))
		return []string{}, err
	}
	return channelIDs, nil
}

// ClearUserAlarms removes all user alarms
func (as *AlarmService) ClearUserAlarms(ctx context.Context, roomID, userID string) (int, error) {
	alarms, err := as.GetUserAlarms(ctx, roomID, userID)
	if err != nil {
		return 0, err
	}

	if len(alarms) == 0 {
		return 0, nil
	}

	alarmKey := as.getAlarmKey(roomID, userID)
	removed, err := as.cache.SRem(ctx, alarmKey, alarms)
	if err != nil {
		as.logger.Error("Failed to clear user alarms", zap.Error(err))
		return 0, err
	}

	registryKey := as.getRegistryKey(roomID, userID)

	// Remove from channel subscribers
	for _, channelID := range alarms {
		channelSubsKey := as.getChannelSubscribersKey(channelID)
		as.cache.SRem(ctx, channelSubsKey, []string{registryKey})

		remainingSubs, err := as.cache.SMembers(ctx, channelSubsKey)
		if err == nil && len(remainingSubs) == 0 {
			as.cache.SRem(ctx, AlarmChannelRegistryKey, []string{channelID})
			as.cache.Del(ctx, channelSubsKey)
		}
	}

	// Remove from registry
	as.cache.SRem(ctx, AlarmRegistryKey, []string{registryKey})

	as.logger.Info("All alarms cleared",
		zap.String("room_id", roomID),
		zap.String("user_id", userID),
		zap.Int("count", int(removed)),
	)

	return int(removed), nil
}

// CheckUpcomingStreams checks for streams starting soon
func (as *AlarmService) CheckUpcomingStreams(ctx context.Context) ([]*domain.AlarmNotification, error) {
	channelIDs, err := as.cache.SMembers(ctx, AlarmChannelRegistryKey)
	if err != nil {
		as.logger.Error("Failed to get channel registry", zap.Error(err))
		return nil, err
	}

	as.logger.Debug("Alarm check started", zap.Int("channels", len(channelIDs)))

	if len(channelIDs) == 0 {
		return []*domain.AlarmNotification{}, nil
	}

	// Process channels concurrently (limit 15)
	p := pool.New().WithMaxGoroutines(as.concurrency)
	now := time.Now()

	results := make([]*channelCheckResult, len(channelIDs))
	resultsMu := sync.Mutex{}

	for idx, channelID := range channelIDs {
		idx, channelID := idx, channelID
		p.Go(func() {
			result := as.checkChannel(ctx, channelID)
			resultsMu.Lock()
			results[idx] = result
			resultsMu.Unlock()
		})
	}

	p.Wait()

	// Process results
	notifications := make([]*domain.AlarmNotification, 0)

	for _, result := range results {
		if result == nil || len(result.subscribers) == 0 {
			continue
		}

		as.logger.Debug("Channel result",
			zap.String("channel_id", result.channelID),
			zap.Int("streams", len(result.streams)),
		)

		// Update next stream cache with mutex protection
		go func(channelID string, streams []*domain.Stream) {
			childCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			as.updateNextStreamCacheFromStreams(childCtx, channelID, streams)
		}(result.channelID, result.streams)

		if len(result.streams) == 0 {
			continue
		}

		// Filter upcoming streams matching target minutes
		upcomingStreams := as.filterUpcomingStreams(result.streams, now)

		as.logger.Debug("Upcoming streams filtered",
			zap.String("channel_id", result.channelID),
			zap.Int("count", len(upcomingStreams)),
		)

		for _, stream := range upcomingStreams {
			roomNotifs, err := as.createNotification(ctx, stream, result.channelID, result.subscribers)
			if err != nil {
				as.logger.Warn("Failed to create notification", zap.Error(err))
				continue
			}

			if len(roomNotifs) > 0 {
				as.logger.Info("Alarm notifications created",
					zap.String("channel", stream.ChannelName),
					zap.Int("minutes_until", roomNotifs[0].MinutesUntil),
					zap.Int("rooms", len(roomNotifs)),
				)
				notifications = append(notifications, roomNotifs...)
			}
		}
	}

	return notifications, nil
}

// channelCheckResult holds the result of checking a channel
type channelCheckResult struct {
	channelID   string
	subscribers []string
	streams     []*domain.Stream
}

// checkChannel checks a single channel for upcoming streams
func (as *AlarmService) checkChannel(ctx context.Context, channelID string) *channelCheckResult {
	channelSubsKey := as.getChannelSubscribersKey(channelID)
	subscribers, err := as.cache.SMembers(ctx, channelSubsKey)
	if err != nil {
		as.logger.Warn("Failed to get subscribers", zap.String("channel_id", channelID), zap.Error(err))
		return &channelCheckResult{channelID: channelID, subscribers: []string{}, streams: []*domain.Stream{}}
	}

	as.logger.Debug("Channel subscribers",
		zap.String("channel_id", channelID),
		zap.Int("count", len(subscribers)),
	)

	if len(subscribers) == 0 {
		as.cache.SRem(ctx, AlarmChannelRegistryKey, []string{channelID})
		as.logger.Debug("Channel removed from registry (no subscribers)", zap.String("channel_id", channelID))
		return &channelCheckResult{channelID: channelID, subscribers: []string{}, streams: []*domain.Stream{}}
	}

	// Get schedule
	streams, err := as.holodex.GetChannelSchedule(ctx, channelID, 24, true)
	if err != nil {
		as.logger.Warn("Failed to get channel schedule",
			zap.String("channel_id", channelID),
			zap.Error(err),
		)
		return &channelCheckResult{channelID: channelID, subscribers: subscribers, streams: []*domain.Stream{}}
	}

	return &channelCheckResult{
		channelID:   channelID,
		subscribers: subscribers,
		streams:     streams,
	}
}

// filterUpcomingStreams filters streams matching target minutes
func (as *AlarmService) filterUpcomingStreams(streams []*domain.Stream, now time.Time) []*domain.Stream {
	filtered := make([]*domain.Stream, 0, len(streams))

	for _, stream := range streams {
		if !stream.IsUpcoming() || stream.StartScheduled == nil {
			continue
		}

		timeUntil := stream.StartScheduled.Sub(now)
		secondsUntil := int(timeUntil.Seconds())
		minutesUntil := secondsUntil / 60

		as.logger.Debug("Stream timing",
			zap.String("stream_id", stream.ID),
			zap.Int("minutes_until", minutesUntil),
			zap.Int("seconds_until", secondsUntil),
		)

		// Check if matches target minutes
		shouldNotify := false
		for _, target := range as.targetMinutes {
			if minutesUntil == target {
				shouldNotify = true
				break
			}
		}

		if secondsUntil > 0 && shouldNotify {
			filtered = append(filtered, stream)
		}
	}

	return filtered
}

// createNotification creates notifications for a stream
func (as *AlarmService) createNotification(ctx context.Context, stream *domain.Stream, channelID string, subscriberKeys []string) ([]*domain.AlarmNotification, error) {
	if stream.StartScheduled == nil {
		as.logger.Debug("No start scheduled", zap.String("stream_id", stream.ID))
		return []*domain.AlarmNotification{}, nil
	}

	now := time.Now()
	timeUntil := stream.StartScheduled.Sub(now)
	secondsUntil := int(timeUntil.Seconds())
	minutesUntil := secondsUntil / 60

	// Check if already notified for this stream
	notifiedKey := NotifiedKeyPrefix + stream.ID
	var notifiedData NotifiedData
	err := as.cache.Get(ctx, notifiedKey, &notifiedData)
	if err == nil && notifiedData.StartScheduled != "" {
		// Parse both times and compare (avoid format mismatch)
		savedTime, err1 := time.Parse(time.RFC3339, notifiedData.StartScheduled)
		currentTime := *stream.StartScheduled

		if err1 == nil && savedTime.Unix() == currentTime.Unix() {
			// Already notified for this exact stream - skip
			as.logger.Debug("Already notified",
				zap.String("stream_id", stream.ID),
				zap.Int("previous_minutes", notifiedData.MinutesUntil),
				zap.Int("current_minutes", minutesUntil),
			)
			return []*domain.AlarmNotification{}, nil
		}
		as.logger.Info("Schedule changed, resetting notification",
			zap.String("stream_id", stream.ID),
			zap.String("old", notifiedData.StartScheduled),
			zap.String("new", stream.StartScheduled.Format(time.RFC3339)),
		)
	}

	// Validate subscribers and group by room
	usersByRoom := make(map[string][]string)
	keysToRemove := make([]string, 0)
	channelSubsKey := as.getChannelSubscribersKey(channelID)

	as.logger.Debug("Validating subscribers",
		zap.String("channel_id", channelID),
		zap.Int("count", len(subscriberKeys)),
	)

	for _, registryKey := range subscriberKeys {
		parts := splitRegistryKey(registryKey)
		if len(parts) != 2 {
			as.logger.Warn("Invalid registry key", zap.String("key", registryKey))
			keysToRemove = append(keysToRemove, registryKey)
			continue
		}

		room, user := parts[0], parts[1]
		userAlarmKey := as.getAlarmKey(room, user)

		// Check if still subscribed
		stillSubscribed, err := as.cache.SIsMember(ctx, userAlarmKey, channelID)
		if err != nil || !stillSubscribed {
			as.logger.Debug("Stale subscriber", zap.String("key", registryKey))
			keysToRemove = append(keysToRemove, registryKey)
			continue
		}

		// Add to room group
		usersByRoom[room] = append(usersByRoom[room], user)
	}

	// Remove stale subscribers
	if len(keysToRemove) > 0 {
		as.cache.SRem(ctx, channelSubsKey, keysToRemove)
		as.logger.Debug("Removed stale subscribers", zap.Int("count", len(keysToRemove)))
	}

	// No valid subscribers
	if len(usersByRoom) == 0 {
		as.logger.Debug("No subscribers, cleaning up", zap.String("channel_id", channelID))
		as.cache.SRem(ctx, AlarmChannelRegistryKey, []string{channelID})
		as.cache.Del(ctx, channelSubsKey)
		return []*domain.AlarmNotification{}, nil
	}

	// Get channel info
	channel, err := as.holodex.GetChannel(ctx, channelID)
	if err != nil || channel == nil {
		as.logger.Warn("Failed to get channel", zap.String("channel_id", channelID), zap.Error(err))
		return []*domain.AlarmNotification{}, nil
	}

	// Create notifications (marking as notified moved to bot.go after successful delivery)
	notifications := make([]*domain.AlarmNotification, 0, len(usersByRoom))
	for roomID, users := range usersByRoom {
		notifications = append(notifications, domain.NewAlarmNotification(
			roomID,
			channel,
			stream,
			minutesUntil,
			users,
		))
		as.logger.Debug("Created notification",
			zap.String("room_id", roomID),
			zap.Int("users", len(users)),
		)
	}

	return notifications, nil
}

// CacheMemberName caches member name for a channel
func (as *AlarmService) CacheMemberName(ctx context.Context, channelID, memberName string) error {
	return as.cache.HSet(ctx, MemberNameKey, channelID, memberName)
}

// GetMemberName gets cached member name
func (as *AlarmService) GetMemberName(ctx context.Context, channelID string) (string, error) {
	return as.cache.HGet(ctx, MemberNameKey, channelID)
}

// MarkAsNotified marks a stream as notified after successful delivery
func (as *AlarmService) MarkAsNotified(ctx context.Context, streamID string, startScheduled time.Time, minutesUntil int) error {
	notifiedKey := NotifiedKeyPrefix + streamID
	notifiedData := NotifiedData{
		StartScheduled: startScheduled.Format(time.RFC3339),
		NotifiedAt:     time.Now().Format(time.RFC3339),
		MinutesUntil:   minutesUntil,
	}

	if err := as.cache.Set(ctx, notifiedKey, notifiedData, 24*time.Hour); err != nil {
		as.logger.Warn("Failed to mark as notified",
			zap.String("stream_id", streamID),
			zap.Error(err),
		)
		return err
	}

	as.logger.Debug("Marked as notified",
		zap.String("stream_id", streamID),
		zap.Int("minutes_until", minutesUntil),
	)
	return nil
}

// GetNextStreamInfo gets next stream info for a channel
func (as *AlarmService) GetNextStreamInfo(ctx context.Context, channelID string) (string, error) {
	// Read lock to prevent reading during cache updates
	as.cacheMutex.RLock()
	defer as.cacheMutex.RUnlock()

	key := NextStreamKeyPrefix + channelID
	data, err := as.cache.HGetAll(ctx, key)
	if err != nil {
		as.logger.Error("Failed to get next stream info from cache",
			zap.String("channel_id", channelID),
			zap.Error(err),
		)
		return "", err
	}

	if len(data) == 0 {
		return "", nil
	}

	status := data["status"]

	// LIVE 스트림 처리
	if status == "live" {
		return "🔴 현재 방송 중!", nil
	}

	// Handle no upcoming or unknown time status
	if status == "no_upcoming" || status == "time_unknown" {
		return "예정된 방송 없음", nil
	}

	// Only upcoming status should have stream data
	if status != "upcoming" {
		as.logger.Warn("Unexpected cache status",
			zap.String("channel_id", channelID),
			zap.String("status", status),
		)
		return "", nil
	}

	// Validate all required fields are present
	title := data["title"]
	startScheduledStr := data["start_scheduled"]
	videoID := data["video_id"]

	if startScheduledStr == "" || title == "" || videoID == "" {
		as.logger.Error("Incomplete cache data for upcoming stream",
			zap.String("channel_id", channelID),
			zap.Bool("has_title", title != ""),
			zap.Bool("has_start", startScheduledStr != ""),
			zap.Bool("has_video_id", videoID != ""),
		)
		return "", nil
	}

	scheduledDate, err := time.Parse(time.RFC3339, startScheduledStr)
	if err != nil {
		as.logger.Error("Failed to parse scheduled time",
			zap.String("channel_id", channelID),
			zap.String("start_scheduled", startScheduledStr),
			zap.Error(err),
		)
		return "", nil
	}

	now := time.Now()
	timeLeft := scheduledDate.Sub(now)

	if timeLeft <= 0 {
		return "방송 시작 임박!", nil
	}

	hoursLeft := int(timeLeft.Hours())
	minutesLeft := int(timeLeft.Minutes()) % 60

	var timeDetail string
	if hoursLeft >= 24 {
		daysLeft := hoursLeft / 24
		timeDetail = fmt.Sprintf("%d일 후", daysLeft)
	} else if hoursLeft > 0 {
		timeDetail = fmt.Sprintf("%d시간 %d분 후", hoursLeft, minutesLeft)
	} else {
		timeDetail = fmt.Sprintf("%d분 후", int(timeLeft.Minutes()))
	}

	// Convert to KST
	kstTime := util.FormatKST(scheduledDate, "01/02 15:04")

	shortTitle := util.TruncateString(title, constants.StringLimits.NextStreamTitle)

	return fmt.Sprintf("다음 방송: %s (%s)\n[%s](https://youtube.com/watch?v=%s)", kstTime, timeDetail, shortTitle, videoID), nil
}

// Helper methods

func (as *AlarmService) updateNextStreamCacheFromStreams(ctx context.Context, channelID string, streams []*domain.Stream) {
	if err := as.updateNextStreamCache(ctx, channelID, streams, "Updated from existing data"); err != nil {
		as.logger.Warn("Failed to update next stream cache", zap.String("channel_id", channelID), zap.Error(err))
	}
}

func (as *AlarmService) updateNextStreamCache(ctx context.Context, channelID string, streams []*domain.Stream, logMsg string) error {
	// Write lock for cache updates
	as.cacheMutex.Lock()
	defer as.cacheMutex.Unlock()

	key := NextStreamKeyPrefix + channelID

	// No streams - set status only (old fields remain but status controls behavior)
	if len(streams) == 0 {
		if err := as.cache.HMSet(ctx, key, map[string]interface{}{"status": "no_upcoming"}); err != nil {
			as.logger.Error("Failed to set no_upcoming status", zap.String("channel_id", channelID), zap.Error(err))
			return err
		}
		if err := as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo); err != nil {
			as.logger.Warn("Failed to set TTL", zap.String("channel_id", channelID), zap.Error(err))
		}
		as.logger.Debug("No streams available", zap.String("channel_id", channelID))
		return nil
	}

	// Find LIVE stream or next upcoming stream
	var liveStream *domain.Stream
	var nextStream *domain.Stream

	for _, s := range streams {
		if s == nil {
			continue
		}
		as.logger.Debug("Checking stream for cache",
			zap.String("stream_id", s.ID),
			zap.String("status", string(s.Status)),
			zap.Bool("is_upcoming", s.IsUpcoming()),
			zap.Bool("is_live", s.IsLive()),
			zap.Bool("has_start", s.StartScheduled != nil),
		)

		// LIVE 스트림 우선
		if s.IsLive() {
			liveStream = s
			break
		}

		// UPCOMING 스트림
		if nextStream == nil && s.IsUpcoming() && s.StartScheduled != nil {
			nextStream = s
		}
	}

	// LIVE 스트림이 있으면 우선 저장
	if liveStream != nil {
		fields := map[string]interface{}{
			"title":    liveStream.Title,
			"video_id": liveStream.ID,
			"status":   "live",
		}
		if liveStream.StartScheduled != nil {
			fields["start_scheduled"] = liveStream.StartScheduled.Format(time.RFC3339)
		}

		if err := as.cache.HMSet(ctx, key, fields); err != nil {
			as.logger.Error("Failed to cache live stream info",
				zap.String("channel_id", channelID),
				zap.String("stream_id", liveStream.ID),
				zap.Error(err),
			)
			return err
		}

		if err := as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo); err != nil {
			as.logger.Warn("Failed to set TTL", zap.String("channel_id", channelID), zap.Error(err))
		}

		as.logger.Debug("Cached live stream", zap.String("channel_id", channelID), zap.String("stream_id", liveStream.ID))
		return nil
	}

	// No valid upcoming stream with start time
	if nextStream == nil || nextStream.StartScheduled == nil {
		// Check if we should preserve existing cache (API instability protection)
		existing, err := as.cache.HGetAll(ctx, key)
		if err == nil && len(existing) > 0 && existing["status"] == "upcoming" {
			cachedVideoID := existing["video_id"]
			if cachedVideoID != "" {
				// Check if the cached stream is still in the upcoming list (just missing StartScheduled)
				for _, s := range streams {
					if s != nil && s.ID == cachedVideoID && s.IsUpcoming() {
						// Same stream, just API didn't return StartScheduled - preserve cache!
						as.logger.Debug("Preserving cache due to API missing StartScheduled",
							zap.String("channel_id", channelID),
							zap.String("stream_id", cachedVideoID),
						)
						// Refresh TTL only
						if err := as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo); err != nil {
							as.logger.Warn("Failed to refresh TTL", zap.String("channel_id", channelID), zap.Error(err))
						}
						return nil
					}
				}
			}
		}

		// No preservation possible - set time_unknown
		if err := as.cache.HMSet(ctx, key, map[string]interface{}{"status": "time_unknown"}); err != nil {
			as.logger.Error("Failed to set time_unknown status", zap.String("channel_id", channelID), zap.Error(err))
			return err
		}
		if err := as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo); err != nil {
			as.logger.Warn("Failed to set TTL", zap.String("channel_id", channelID), zap.Error(err))
		}
		as.logger.Debug("No valid upcoming stream found", zap.String("channel_id", channelID))
		return nil
	}

	// Set all fields atomically using HMSet
	fields := map[string]interface{}{
		"title":           nextStream.Title,
		"start_scheduled": nextStream.StartScheduled.Format(time.RFC3339),
		"video_id":        nextStream.ID,
		"status":          "upcoming",
	}

	if err := as.cache.HMSet(ctx, key, fields); err != nil {
		as.logger.Error("Failed to cache next stream info",
			zap.String("channel_id", channelID),
			zap.String("stream_id", nextStream.ID),
			zap.Error(err),
		)
		return err
	}

	if err := as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo); err != nil {
		as.logger.Warn("Failed to set TTL", zap.String("channel_id", channelID), zap.Error(err))
	}

	as.logger.Debug(logMsg,
		zap.String("channel_id", channelID),
		zap.String("stream_id", nextStream.ID),
		zap.String("title", nextStream.Title),
	)
	return nil
}

func (as *AlarmService) getAlarmKey(roomID, userID string) string {
	return AlarmKeyPrefix + roomID + ":" + userID
}

func (as *AlarmService) getRegistryKey(roomID, userID string) string {
	return roomID + ":" + userID
}

func (as *AlarmService) getChannelSubscribersKey(channelID string) string {
	return ChannelSubscribersKeyPrefix + channelID
}

// Helper functions

func splitRegistryKey(key string) []string {
	return strings.SplitN(key, ":", 2)
}
