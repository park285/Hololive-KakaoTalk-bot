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

const (
	AlarmKeyPrefix              = "alarm:"
	AlarmRegistryKey            = "alarm:registry"
	AlarmChannelRegistryKey     = "alarm:channel_registry"
	ChannelSubscribersKeyPrefix = "alarm:channel_subscribers:"
	MemberNameKey               = "member_names"
	NotifiedKeyPrefix           = "notified:"
	NextStreamKeyPrefix         = "alarm:next_stream:"
)

type NotifiedData struct {
	StartScheduled string `json:"start_scheduled"`
	NotifiedAt     string `json:"notified_at"`
	MinutesUntil   int    `json:"minutes_until"`
}

type AlarmService struct {
	cache         *CacheService
	holodex       *HolodexService
	logger        *zap.Logger
	targetMinutes []int
	concurrency   int
	cacheMutex    sync.RWMutex
}

func NewAlarmService(cache *CacheService, holodex *HolodexService, logger *zap.Logger, advanceMinutes []int) *AlarmService {
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

func (as *AlarmService) AddAlarm(ctx context.Context, roomID, userID, channelID, memberName string) (bool, error) {
	alarmKey := as.getAlarmKey(roomID, userID)
	added, err := as.cache.SAdd(ctx, alarmKey, []string{channelID})
	if err != nil {
		as.logger.Error("Failed to add alarm", zap.Error(err))
		return false, err
	}

	registryKey := as.getRegistryKey(roomID, userID)
	if _, err := as.cache.SAdd(ctx, AlarmRegistryKey, []string{registryKey}); err != nil {
		as.logger.Warn("Failed to add to registry", zap.Error(err))
	}

	channelSubsKey := as.getChannelSubscribersKey(channelID)
	if _, err := as.cache.SAdd(ctx, channelSubsKey, []string{registryKey}); err != nil {
		as.logger.Warn("Failed to add channel subscriber", zap.Error(err))
	}

	if _, err := as.cache.SAdd(ctx, AlarmChannelRegistryKey, []string{channelID}); err != nil {
		as.logger.Warn("Failed to add to channel registry", zap.Error(err))
	}

	if err := as.CacheMemberName(ctx, channelID, memberName); err != nil {
		as.logger.Warn("Failed to cache member name", zap.Error(err))
	}

	as.logger.Info("Alarm added",
		zap.String("room_id", roomID),
		zap.String("user_id", userID),
		zap.String("channel_id", channelID),
		zap.String("member_name", memberName),
	)

	return added > 0, nil
}

func (as *AlarmService) RemoveAlarm(ctx context.Context, roomID, userID, channelID string) (bool, error) {
	alarmKey := as.getAlarmKey(roomID, userID)
	removed, err := as.cache.SRem(ctx, alarmKey, []string{channelID})
	if err != nil {
		as.logger.Error("Failed to remove alarm", zap.Error(err))
		return false, err
	}

	registryKey := as.getRegistryKey(roomID, userID)
	channelSubsKey := as.getChannelSubscribersKey(channelID)

	if _, err := as.cache.SRem(ctx, channelSubsKey, []string{registryKey}); err != nil {
		as.logger.Warn("Failed to remove from channel subscribers", zap.Error(err))
	}

	remainingSubs, err := as.cache.SMembers(ctx, channelSubsKey)
	if err == nil && len(remainingSubs) == 0 {
		as.cache.SRem(ctx, AlarmChannelRegistryKey, []string{channelID})
		as.cache.Del(ctx, channelSubsKey)
	}

	remainingAlarms, err := as.cache.SMembers(ctx, alarmKey)
	if err == nil && len(remainingAlarms) == 0 {
		as.cache.SRem(ctx, AlarmRegistryKey, []string{registryKey})
		as.logger.Info("User removed from registry (no alarms left)",
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

func (as *AlarmService) GetUserAlarms(ctx context.Context, roomID, userID string) ([]string, error) {
	alarmKey := as.getAlarmKey(roomID, userID)
	channelIDs, err := as.cache.SMembers(ctx, alarmKey)
	if err != nil {
		as.logger.Error("Failed to get user alarms", zap.Error(err))
		return []string{}, err
	}
	return channelIDs, nil
}

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

	for _, channelID := range alarms {
		channelSubsKey := as.getChannelSubscribersKey(channelID)
		as.cache.SRem(ctx, channelSubsKey, []string{registryKey})

		remainingSubs, err := as.cache.SMembers(ctx, channelSubsKey)
		if err == nil && len(remainingSubs) == 0 {
			as.cache.SRem(ctx, AlarmChannelRegistryKey, []string{channelID})
			as.cache.Del(ctx, channelSubsKey)
		}
	}

	as.cache.SRem(ctx, AlarmRegistryKey, []string{registryKey})

	as.logger.Info("All alarms cleared",
		zap.String("room_id", roomID),
		zap.String("user_id", userID),
		zap.Int("count", int(removed)),
	)

	return int(removed), nil
}

func (as *AlarmService) CheckUpcomingStreams(ctx context.Context) ([]*domain.AlarmNotification, error) {
	channelIDs, err := as.cache.SMembers(ctx, AlarmChannelRegistryKey)
	if err != nil {
		as.logger.Error("Failed to get channel registry", zap.Error(err))
		return nil, err
	}

	if len(channelIDs) == 0 {
		return []*domain.AlarmNotification{}, nil
	}
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

	notifications := make([]*domain.AlarmNotification, 0)

	for _, result := range results {
		if result == nil || len(result.subscribers) == 0 {
			continue
		}

		as.triggerCacheRefresh(ctx, result.channelID, result.streams)

		if len(result.streams) == 0 {
			continue
		}

		upcomingStreams := as.filterUpcomingStreams(result.streams, now)

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

type channelCheckResult struct {
	channelID   string
	subscribers []string
	streams     []*domain.Stream
}

func (as *AlarmService) checkChannel(ctx context.Context, channelID string) *channelCheckResult {
	channelSubsKey := as.getChannelSubscribersKey(channelID)
	subscribers, err := as.cache.SMembers(ctx, channelSubsKey)
	if err != nil {
		as.logger.Warn("Failed to get subscribers", zap.String("channel_id", channelID), zap.Error(err))
		return &channelCheckResult{channelID: channelID, subscribers: []string{}, streams: []*domain.Stream{}}
	}

	// 채널 구독자 수 로그 (필요시 활성화)
	// as.logger.Info("Channel subscribers", zap.String("channel_id", channelID), zap.Int("count", len(subscribers)))

	if len(subscribers) == 0 {
		as.cache.SRem(ctx, AlarmChannelRegistryKey, []string{channelID})
		as.logger.Info("Channel removed from registry (no subscribers)", zap.String("channel_id", channelID))
		return &channelCheckResult{channelID: channelID, subscribers: []string{}, streams: []*domain.Stream{}}
	}

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

func (as *AlarmService) filterUpcomingStreams(streams []*domain.Stream, now time.Time) []*domain.Stream {
	filtered := make([]*domain.Stream, 0, len(streams))

	for _, stream := range streams {
		if !stream.IsUpcoming() || stream.StartScheduled == nil {
			continue
		}

		timeUntil := stream.StartScheduled.Sub(now)
		secondsUntil := int(timeUntil.Seconds())
		minutesUntil := secondsUntil / 60

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

func (as *AlarmService) triggerCacheRefresh(parent context.Context, channelID string, streams []*domain.Stream) {
	if parent == nil {
		return
	}

	select {
	case <-parent.Done():
		return
	default:
	}

	go func(p context.Context, chID string, data []*domain.Stream) {
		ctxWithTimeout, cancel := context.WithTimeout(p, 10*time.Second)
		defer cancel()
		as.updateNextStreamCacheFromStreams(ctxWithTimeout, chID, data)
	}(parent, channelID, streams)
}

func (as *AlarmService) createNotification(ctx context.Context, stream *domain.Stream, channelID string, subscriberKeys []string) ([]*domain.AlarmNotification, error) {
	if stream.StartScheduled == nil {
		return []*domain.AlarmNotification{}, nil
	}

	now := time.Now()
	timeUntil := stream.StartScheduled.Sub(now)
	secondsUntil := int(timeUntil.Seconds())
	minutesUntil := secondsUntil / 60

	notifiedKey := NotifiedKeyPrefix + stream.ID
	var notifiedData NotifiedData
	err := as.cache.Get(ctx, notifiedKey, &notifiedData)
	if err == nil && notifiedData.StartScheduled != "" {
		savedTime, err1 := time.Parse(time.RFC3339, notifiedData.StartScheduled)
		currentTime := *stream.StartScheduled

		if err1 == nil && savedTime.Unix() == currentTime.Unix() {
			return []*domain.AlarmNotification{}, nil
		}
		as.logger.Info("Schedule changed, resetting notification",
			zap.String("stream_id", stream.ID),
			zap.String("old", notifiedData.StartScheduled),
			zap.String("new", stream.StartScheduled.Format(time.RFC3339)),
		)
	}

	usersByRoom := make(map[string][]string)
	keysToRemove := make([]string, 0)
	channelSubsKey := as.getChannelSubscribersKey(channelID)

	for _, registryKey := range subscriberKeys {
		parts := splitRegistryKey(registryKey)
		if len(parts) != 2 {
			as.logger.Warn("Invalid registry key", zap.String("key", registryKey))
			keysToRemove = append(keysToRemove, registryKey)
			continue
		}

		room, user := parts[0], parts[1]
		userAlarmKey := as.getAlarmKey(room, user)

		stillSubscribed, err := as.cache.SIsMember(ctx, userAlarmKey, channelID)
		if err != nil || !stillSubscribed {
			keysToRemove = append(keysToRemove, registryKey)
			continue
		}

		usersByRoom[room] = append(usersByRoom[room], user)
	}

	if len(keysToRemove) > 0 {
		as.cache.SRem(ctx, channelSubsKey, keysToRemove)
	}

	if len(usersByRoom) == 0 {
		as.cache.SRem(ctx, AlarmChannelRegistryKey, []string{channelID})
		as.cache.Del(ctx, channelSubsKey)
		return []*domain.AlarmNotification{}, nil
	}

	channel, err := as.holodex.GetChannel(ctx, channelID)
	if err != nil || channel == nil {
		as.logger.Warn("Failed to get channel", zap.String("channel_id", channelID), zap.Error(err))
		return []*domain.AlarmNotification{}, nil
	}

	notifications := make([]*domain.AlarmNotification, 0, len(usersByRoom))
	for roomID, users := range usersByRoom {
		notifications = append(notifications, domain.NewAlarmNotification(
			roomID,
			channel,
			stream,
			minutesUntil,
			users,
		))
	}

	return notifications, nil
}

func (as *AlarmService) CacheMemberName(ctx context.Context, channelID, memberName string) error {
	return as.cache.HSet(ctx, MemberNameKey, channelID, memberName)
}

func (as *AlarmService) GetMemberName(ctx context.Context, channelID string) (string, error) {
	return as.cache.HGet(ctx, MemberNameKey, channelID)
}

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

	return nil
}

func (as *AlarmService) GetNextStreamInfo(ctx context.Context, channelID string) (string, error) {
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

	if status == "live" {
		return "   🔴 현재 방송 중!", nil
	}

	if status == "no_upcoming" || status == "time_unknown" {
		return "   예정된 방송 없음", nil
	}

	if status != "upcoming" {
		as.logger.Warn("Unexpected cache status",
			zap.String("channel_id", channelID),
			zap.String("status", status),
		)
		return "", nil
	}

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

	kstTime := util.FormatKST(scheduledDate, "01/02 15:04")

	shortTitle := util.TruncateString(title, constants.StringLimits.NextStreamTitle)

	return fmt.Sprintf("   다음 방송: %s (%s)\n   [%s](https://youtube.com/watch?v=%s)", kstTime, timeDetail, shortTitle, videoID), nil
}

func (as *AlarmService) updateNextStreamCacheFromStreams(ctx context.Context, channelID string, streams []*domain.Stream) {
	if err := as.updateNextStreamCache(ctx, channelID, streams, "Updated from existing data"); err != nil {
		as.logger.Warn("Failed to update next stream cache", zap.String("channel_id", channelID), zap.Error(err))
	}
}

func (as *AlarmService) findLiveStream(streams []*domain.Stream) *domain.Stream {
	for _, s := range streams {
		if s != nil && s.IsLive() {
			return s
		}
	}
	return nil
}

func (as *AlarmService) findNextUpcomingStream(streams []*domain.Stream) *domain.Stream {
	for _, s := range streams {
		if s != nil && s.IsUpcoming() && s.StartScheduled != nil {
			return s
		}
	}
	return nil
}

func (as *AlarmService) cacheLiveStream(ctx context.Context, key string, stream *domain.Stream) error {
	fields := map[string]interface{}{
		"title":    stream.Title,
		"video_id": stream.ID,
		"status":   "live",
	}
	if stream.StartScheduled != nil {
		fields["start_scheduled"] = stream.StartScheduled.Format(time.RFC3339)
	}

	if err := as.cache.HMSet(ctx, key, fields); err != nil {
		as.logger.Error("Failed to cache live stream", zap.String("stream_id", stream.ID), zap.Error(err))
		return err
	}

	as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo)
	return nil
}

func (as *AlarmService) cacheUpcomingStream(ctx context.Context, key string, stream *domain.Stream, logMsg string) error {
	fields := map[string]interface{}{
		"title":           stream.Title,
		"start_scheduled": stream.StartScheduled.Format(time.RFC3339),
		"video_id":        stream.ID,
		"status":          "upcoming",
	}

	if err := as.cache.HMSet(ctx, key, fields); err != nil {
		as.logger.Error("Failed to cache upcoming stream", zap.String("stream_id", stream.ID), zap.Error(err))
		return err
	}

	as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo)
	return nil
}

func (as *AlarmService) cacheStatus(ctx context.Context, key, status string) error {
	if err := as.cache.HMSet(ctx, key, map[string]interface{}{"status": status}); err != nil {
		as.logger.Error("Failed to set cache status", zap.String("status", status), zap.Error(err))
		return err
	}

	as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo)
	return nil
}

func (as *AlarmService) shouldPreserveCache(ctx context.Context, key string, streams []*domain.Stream) bool {
	existing, err := as.cache.HGetAll(ctx, key)
	if err != nil || len(existing) == 0 || existing["status"] != "upcoming" {
		return false
	}

	cachedVideoID := existing["video_id"]
	if cachedVideoID == "" {
		return false
	}

	for _, s := range streams {
		if s != nil && s.ID == cachedVideoID && s.IsUpcoming() {
			as.cache.Expire(ctx, key, constants.CacheTTL.NextStreamInfo)
			return true
		}
	}

	return false
}

func (as *AlarmService) updateNextStreamCache(ctx context.Context, channelID string, streams []*domain.Stream, logMsg string) error {
	as.cacheMutex.Lock()
	defer as.cacheMutex.Unlock()

	key := NextStreamKeyPrefix + channelID

	if len(streams) == 0 {
		return as.cacheStatus(ctx, key, "no_upcoming")
	}

	if liveStream := as.findLiveStream(streams); liveStream != nil {
		return as.cacheLiveStream(ctx, key, liveStream)
	}

	upcomingStream := as.findNextUpcomingStream(streams)

	if upcomingStream == nil || upcomingStream.StartScheduled == nil {
		if as.shouldPreserveCache(ctx, key, streams) {
			return nil
		}
		return as.cacheStatus(ctx, key, "time_unknown")
	}
	return as.cacheUpcomingStream(ctx, key, upcomingStream, logMsg)
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

func splitRegistryKey(key string) []string {
	return strings.SplitN(key, ":", 2)
}
