package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"github.com/kapu/hololive-kakao-bot-go/pkg/errors"
	"go.uber.org/zap"
)

type HolodexChannelRaw struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	EnglishName     *string `json:"english_name,omitempty"`
	Photo           *string `json:"photo,omitempty"`
	Twitter         *string `json:"twitter,omitempty"`
	VideoCount      *int    `json:"video_count,omitempty"`
	SubscriberCount *int    `json:"subscriber_count,omitempty"`
	Org             *string `json:"org,omitempty"`
	Suborg          *string `json:"suborg,omitempty"`
	Group           *string `json:"group,omitempty"`
}

type HolodexStreamRaw struct {
	ID             string              `json:"id"`
	Title          string              `json:"title"`
	ChannelID      *string             `json:"channel_id,omitempty"`
	Status         domain.StreamStatus `json:"status"`
	StartScheduled *string             `json:"start_scheduled,omitempty"`
	StartActual    *string             `json:"start_actual,omitempty"`
	Duration       *int                `json:"duration,omitempty"`
	Link           *string             `json:"link,omitempty"`
	Thumbnail      *string             `json:"thumbnail,omitempty"`
	TopicID        *string             `json:"topic_id,omitempty"`
	Channel        *HolodexChannelRaw  `json:"channel,omitempty"`
}

type HolodexService struct {
	requester HolodexRequester
	cache     *CacheService
	scraper   *ScraperService // Fallback scraper
	logger    *zap.Logger
}

func NewHolodexService(apiKeys []string, cache *CacheService, scraper *ScraperService, logger *zap.Logger) (*HolodexService, error) {
	if len(apiKeys) == 0 {
		return nil, fmt.Errorf("at least one Holodex API key is required")
	}

	httpClient := &http.Client{
		Timeout: constants.APIConfig.HolodexTimeout,
	}

	requester := NewHolodexAPIClient(httpClient, apiKeys, logger)

	return &HolodexService{
		requester: requester,
		cache:     cache,
		scraper:   scraper,
		logger:    logger,
	}, nil
}

func (h *HolodexService) GetLiveStreams(ctx context.Context) ([]*domain.Stream, error) {
	cacheKey := "live_streams"

	var cached []*domain.Stream
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil && cached != nil {
		return cached, nil
	}

	params := url.Values{}
	params.Set("org", "Hololive")
	params.Set("status", "live")
	params.Set("type", "stream")

	body, err := h.requester.DoRequest(ctx, "GET", "/live", params)
	if err != nil {
		h.logger.Error("Failed to get live streams", zap.Error(err))
		return nil, err
	}

	var rawStreams []HolodexStreamRaw
	if err := json.Unmarshal(body, &rawStreams); err != nil {
		return nil, err
	}

	streams := h.mapStreamsResponse(rawStreams)
	filtered := h.filterHololiveStreams(streams)

	_ = h.cache.Set(ctx, cacheKey, filtered, constants.CacheTTL.LiveStreams)

	return filtered, nil
}

func (h *HolodexService) GetUpcomingStreams(ctx context.Context, hours int) ([]*domain.Stream, error) {
	cacheKey := fmt.Sprintf("upcoming_streams_%d", hours)

	var cached []*domain.Stream
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil && cached != nil {
		return cached, nil
	}

	params := url.Values{}
	params.Set("org", "Hololive")
	params.Set("status", "upcoming")
	params.Set("type", "stream")
	params.Set("max_upcoming_hours", fmt.Sprintf("%d", util.Min(hours, 168)))
	params.Set("order", "asc")
	params.Set("orderby", "start_scheduled")

	body, err := h.requester.DoRequest(ctx, "GET", "/live", params)
	if err != nil {
		h.logger.Error("Failed to get upcoming streams", zap.Error(err))
		return nil, err
	}

	var rawStreams []HolodexStreamRaw
	if err := json.Unmarshal(body, &rawStreams); err != nil {
		return nil, err
	}

	streams := h.mapStreamsResponse(rawStreams)
	filtered := h.filterHololiveStreams(streams)
	upcoming := h.filterUpcomingStreams(filtered)

	_ = h.cache.Set(ctx, cacheKey, upcoming, constants.CacheTTL.UpcomingStreams)

	return upcoming, nil
}

func (h *HolodexService) GetChannelSchedule(ctx context.Context, channelID string, hours int, includeLive bool) ([]*domain.Stream, error) {
	cacheKey := fmt.Sprintf("channel_schedule_%s_%d_%t", channelID, hours, includeLive)

	var cached []*domain.Stream
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil && cached != nil {
		copied := make([]*domain.Stream, len(cached))
		for i, stream := range cached {
			streamCopy := *stream
			if stream.StartScheduled != nil {
				t := *stream.StartScheduled
				streamCopy.StartScheduled = &t
			}
			if stream.StartActual != nil {
				t := *stream.StartActual
				streamCopy.StartActual = &t
			}
			copied[i] = &streamCopy
		}

		if includeLive {
			return copied, nil
		}
		return h.filterUpcomingStreams(copied), nil
	}

	var statuses []domain.StreamStatus
	if includeLive {
		statuses = []domain.StreamStatus{domain.StreamStatusLive, domain.StreamStatusUpcoming}
	} else {
		statuses = []domain.StreamStatus{domain.StreamStatusUpcoming}
	}

	allStreams := make([]*domain.Stream, 0)

	for _, status := range statuses {
		params := url.Values{}
		params.Set("channel_id", channelID)
		params.Set("status", string(status))
		params.Set("type", "stream")
		params.Set("max_upcoming_hours", fmt.Sprintf("%d", hours))

		body, err := h.requester.DoRequest(ctx, "GET", "/live", params)
		if err != nil {
			h.logger.Error("Failed to get channel schedule",
				zap.String("channel_id", channelID),
				zap.String("status", string(status)),
				zap.Error(err),
			)

			if h.shouldUseFallback(err) && h.scraper != nil {
				h.logger.Warn("Using scraper fallback for channel schedule",
					zap.String("channel_id", channelID),
					zap.Error(err))

				return h.scraper.FetchChannel(ctx, channelID)
			}

			return nil, err
		}

		var rawStreams []HolodexStreamRaw
		if err := json.Unmarshal(body, &rawStreams); err != nil {
			return nil, err
		}

		streams := h.mapStreamsResponse(rawStreams)
		allStreams = append(allStreams, streams...)
	}

	hololiveOnly := h.filterHololiveStreams(allStreams)

	sort.Slice(hololiveOnly, func(i, j int) bool {
		iTime := int64(0)
		jTime := int64(0)
		if hololiveOnly[i].StartScheduled != nil {
			iTime = hololiveOnly[i].StartScheduled.Unix()
		}
		if hololiveOnly[j].StartScheduled != nil {
			jTime = hololiveOnly[j].StartScheduled.Unix()
		}
		return iTime < jTime
	})

	result := hololiveOnly
	if !includeLive {
		result = h.filterUpcomingStreams(hololiveOnly)
	}

	_ = h.cache.Set(ctx, cacheKey, result, constants.CacheTTL.ChannelSchedule)

	return result, nil
}

func (h *HolodexService) SearchChannels(ctx context.Context, query string) ([]*domain.Channel, error) {
	cacheKey := fmt.Sprintf("search_channels_%s", query)

	var cached []*domain.Channel
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil && cached != nil {
		return cached, nil
	}

	params := url.Values{}
	params.Set("org", "Hololive")
	params.Set("name", query)
	params.Set("limit", "50")

	body, err := h.requester.DoRequest(ctx, "GET", "/channels", params)
	if err != nil {
		h.logger.Error("Failed to search channels", zap.String("query", query), zap.Error(err))
		return nil, err
	}

	var rawChannels []HolodexChannelRaw
	if err := json.Unmarshal(body, &rawChannels); err != nil {
		return nil, err
	}

	channels := h.mapChannelsResponse(rawChannels)

	h.logger.Debug("Holodex API search results",
		zap.String("query", query),
		zap.Int("total_results", len(channels)),
	)

	filtered := make([]*domain.Channel, 0, len(channels))
	for _, ch := range channels {
		if ch.Org != nil && *ch.Org == "Hololive" && !h.isHolostarsChannel(ch) {
			filtered = append(filtered, ch)
		}
	}

	h.logger.Debug("After HOLOSTARS filter", zap.Int("count", len(filtered)))

	_ = h.cache.Set(ctx, cacheKey, filtered, constants.CacheTTL.ChannelSearch)

	return filtered, nil
}

func (h *HolodexService) GetChannel(ctx context.Context, channelID string) (*domain.Channel, error) {
	cacheKey := fmt.Sprintf("channel_%s", channelID)

	var cached domain.Channel
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil && cached.ID != "" {
		return &cached, nil
	}

	body, err := h.requester.DoRequest(ctx, "GET", "/channels/"+channelID, nil)
	if err != nil {
		if apiErr, ok := err.(*errors.APIError); ok && apiErr.StatusCode == 404 {
			return nil, nil
		}
		h.logger.Error("Failed to get channel", zap.String("channel_id", channelID), zap.Error(err))
		return nil, err
	}

	var rawChannel HolodexChannelRaw
	if err := json.Unmarshal(body, &rawChannel); err != nil {
		return nil, err
	}

	channel := h.mapChannelResponse(&rawChannel)
	_ = h.cache.Set(ctx, cacheKey, channel, constants.CacheTTL.ChannelInfo)

	return channel, nil
}

func (h *HolodexService) mapStreamsResponse(rawStreams []HolodexStreamRaw) []*domain.Stream {
	streams := make([]*domain.Stream, 0, len(rawStreams))
	for _, raw := range rawStreams {
		stream := h.mapStreamResponse(&raw)
		if stream != nil {
			streams = append(streams, stream)
		}
	}
	return streams
}

func (h *HolodexService) mapStreamResponse(raw *HolodexStreamRaw) *domain.Stream {
	stream := &domain.Stream{
		ID:        raw.ID,
		Title:     raw.Title,
		Status:    raw.Status,
		Duration:  raw.Duration,
		Thumbnail: raw.Thumbnail,
		Link:      raw.Link,
		TopicID:   raw.TopicID,
	}

	// ChannelID 설정 (빈 문자열 체크)
	if raw.ChannelID != nil && *raw.ChannelID != "" {
		stream.ChannelID = *raw.ChannelID
	} else if raw.Channel != nil && raw.Channel.ID != "" {
		stream.ChannelID = raw.Channel.ID
	} else {
		// ChannelID가 없으면 invalid 데이터로 간주
		h.logger.Warn("Stream missing ChannelID - skipping",
			zap.String("stream_id", raw.ID),
			zap.String("title", raw.Title))
		return nil
	}

	// ChannelName 설정 (빈 문자열 체크)
	if raw.Channel != nil && raw.Channel.Name != "" {
		stream.ChannelName = raw.Channel.Name
	} else {
		// ChannelName이 없어도 ChannelID가 있으면 허용
		h.logger.Debug("Stream missing ChannelName, will use ChannelID",
			zap.String("stream_id", raw.ID),
			zap.String("channel_id", stream.ChannelID))
	}

	if raw.StartScheduled != nil && *raw.StartScheduled != "" {
		if t, err := time.Parse(time.RFC3339, *raw.StartScheduled); err == nil {
			stream.StartScheduled = &t
		}
	}

	if raw.StartActual != nil && *raw.StartActual != "" {
		if t, err := time.Parse(time.RFC3339, *raw.StartActual); err == nil {
			stream.StartActual = &t
		}
	}

	if raw.Channel != nil {
		stream.Channel = h.mapChannelResponse(raw.Channel)
	}

	return stream
}

func (h *HolodexService) mapChannelsResponse(rawChannels []HolodexChannelRaw) []*domain.Channel {
	channels := make([]*domain.Channel, len(rawChannels))
	for i, raw := range rawChannels {
		channels[i] = h.mapChannelResponse(&raw)
	}
	return channels
}

func (h *HolodexService) mapChannelResponse(raw *HolodexChannelRaw) *domain.Channel {
	return &domain.Channel{
		ID:              raw.ID,
		Name:            raw.Name,
		EnglishName:     raw.EnglishName,
		Photo:           raw.Photo,
		Twitter:         raw.Twitter,
		VideoCount:      raw.VideoCount,
		SubscriberCount: raw.SubscriberCount,
		Org:             raw.Org,
		Suborg:          raw.Suborg,
		Group:           raw.Group,
	}
}

func (h *HolodexService) filterHololiveStreams(streams []*domain.Stream) []*domain.Stream {
	filtered := make([]*domain.Stream, 0, len(streams))

	for _, stream := range streams {
		if stream.Channel == nil {
			h.logger.Debug("Filtered out stream without channel info", zap.String("id", stream.ID))
			continue
		}

		channel := stream.Channel

		if channel.Org == nil || *channel.Org != "Hololive" {
			org := ""
			if channel.Org != nil {
				org = *channel.Org
			}
			h.logger.Debug("Filtered out non-Hololive stream",
				zap.String("channel", stream.ChannelName),
				zap.String("org", org),
			)
			continue
		}

		if h.isHolostarsChannel(channel) {
			h.logger.Debug("Filtered out HOLOSTARS stream", zap.String("channel", stream.ChannelName))
			continue
		}

		filtered = append(filtered, stream)
	}

	return filtered
}

func (h *HolodexService) filterUpcomingStreams(streams []*domain.Stream) []*domain.Stream {
	now := time.Now()
	filtered := make([]*domain.Stream, 0, len(streams))

	for _, stream := range streams {
		if stream.StartActual != nil {
			continue
		}

		if stream.StartScheduled != nil && stream.StartScheduled.After(now) {
			filtered = append(filtered, stream)
		} else if stream.StartScheduled == nil {
			filtered = append(filtered, stream)
		}
	}

	return filtered
}

func (h *HolodexService) isHolostarsChannel(channel *domain.Channel) bool {
	if channel == nil {
		return false
	}

	upper := func(s *string) string {
		if s == nil {
			return ""
		}
		return strings.ToUpper(*s)
	}

	return strings.Contains(upper(channel.Suborg), "HOLOSTARS") ||
		strings.Contains(strings.ToUpper(channel.Name), "HOLOSTARS") ||
		strings.Contains(upper(channel.EnglishName), "HOLOSTARS")
}

func (h *HolodexService) shouldUseFallback(err error) bool {
	if err == nil {
		return false
	}

	if h.requester != nil && h.requester.IsCircuitOpen() {
		return true
	}

	if apiErr, ok := err.(*errors.APIError); ok {
		if apiErr.StatusCode >= 500 {
			return true
		}
		if apiErr.StatusCode == 503 {
			return true
		}
	}

	if _, ok := err.(*errors.KeyRotationError); ok {
		return true
	}

	return false
}
