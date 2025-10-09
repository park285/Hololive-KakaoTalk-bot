package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"github.com/kapu/hololive-kakao-bot-go/pkg/errors"
	"go.uber.org/zap"
)

// HolodexChannelRaw represents the raw Holodex API channel response
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

// HolodexStreamRaw represents the raw Holodex API stream response
type HolodexStreamRaw struct {
	ID             string             `json:"id"`
	Title          string             `json:"title"`
	ChannelID      *string            `json:"channel_id,omitempty"`
	Status         domain.StreamStatus `json:"status"`
	StartScheduled *string            `json:"start_scheduled,omitempty"`
	StartActual    *string            `json:"start_actual,omitempty"`
	Duration       *int               `json:"duration,omitempty"`
	Link           *string            `json:"link,omitempty"`
	Thumbnail      *string            `json:"thumbnail,omitempty"`
	TopicID        *string            `json:"topic_id,omitempty"`
	Channel        *HolodexChannelRaw `json:"channel,omitempty"`
}

// HolodexService provides access to Holodex API
type HolodexService struct {
	httpClient       *http.Client
	apiKeys          []string
	currentKeyIndex  int
	keyMu            sync.Mutex
	cache            *CacheService
	scraper          *ScraperService // Fallback scraper
	logger           *zap.Logger
	failureCount     int
	failureMu        sync.Mutex // Protects failureCount
	circuitOpenUntil *time.Time
	circuitMu        sync.RWMutex
}

// NewHolodexService creates a new Holodex service with optional scraper fallback
func NewHolodexService(apiKeys []string, cache *CacheService, scraper *ScraperService, logger *zap.Logger) (*HolodexService, error) {
	if len(apiKeys) == 0 {
		return nil, fmt.Errorf("at least one Holodex API key is required")
	}

	return &HolodexService{
		httpClient: &http.Client{
			Timeout: constants.APIConfig.HolodexTimeout,
		},
		apiKeys:         apiKeys,
		currentKeyIndex: 0,
		cache:           cache,
		scraper:         scraper,
		logger:          logger,
		failureCount:    0,
	}, nil
}

// getNextAPIKey gets the next API key in rotation
func (h *HolodexService) getNextAPIKey() string {
	h.keyMu.Lock()
	defer h.keyMu.Unlock()

	key := h.apiKeys[h.currentKeyIndex]
	h.currentKeyIndex = (h.currentKeyIndex + 1) % len(h.apiKeys)
	return key
}

// isCircuitOpen checks if circuit breaker is open
func (h *HolodexService) isCircuitOpen() bool {
	h.circuitMu.RLock()
	defer h.circuitMu.RUnlock()

	if h.circuitOpenUntil == nil {
		return false
	}

	if time.Now().After(*h.circuitOpenUntil) {
		return false
	}

	return true
}

// openCircuit opens the circuit breaker
func (h *HolodexService) openCircuit() {
	h.circuitMu.Lock()
	defer h.circuitMu.Unlock()

	resetTime := time.Now().Add(constants.CircuitBreakerConfig.ResetTimeout)
	h.circuitOpenUntil = &resetTime
	h.failureCount = 0

	h.logger.Error("Holodex circuit breaker opened",
		zap.Duration("reset_timeout", constants.CircuitBreakerConfig.ResetTimeout),
	)
}

// resetCircuit resets the circuit breaker
func (h *HolodexService) resetCircuit() {
	h.circuitMu.Lock()
	defer h.circuitMu.Unlock()

	h.failureMu.Lock()
	h.failureCount = 0
	h.failureMu.Unlock()

	h.circuitOpenUntil = nil
}

// incrementFailureCount safely increments the failure counter
func (h *HolodexService) incrementFailureCount() int {
	h.failureMu.Lock()
	defer h.failureMu.Unlock()
	h.failureCount++
	return h.failureCount
}

// getFailureCount safely gets the current failure count
func (h *HolodexService) getFailureCount() int {
	h.failureMu.Lock()
	defer h.failureMu.Unlock()
	return h.failureCount
}

// computeDelay computes exponential backoff delay with jitter
func (h *HolodexService) computeDelay(attempt int) time.Duration {
	jitter := time.Duration(rand.Float64() * float64(constants.RetryConfig.Jitter))
	base := constants.RetryConfig.BaseDelay * time.Duration(math.Pow(2, float64(attempt)))
	return base + jitter
}

// doRequest performs an HTTP request with retry and circuit breaker
func (h *HolodexService) doRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	// Circuit breaker check
	if h.isCircuitOpen() {
		h.circuitMu.RLock()
		var remainingMs int64
		if h.circuitOpenUntil != nil {
			remainingMs = time.Until(*h.circuitOpenUntil).Milliseconds()
		}
		h.circuitMu.RUnlock()

		h.logger.Warn("Circuit breaker is open", zap.Int64("retry_after_ms", remainingMs))
		return nil, errors.NewAPIError("Circuit breaker open", 503, map[string]any{
			"retry_after_ms": remainingMs,
		})
	}

	maxAttempts := util.Min(len(h.apiKeys)*2, 10)
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		apiKey := h.getNextAPIKey()

		reqURL := constants.APIConfig.HolodexBaseURL + path
		if params != nil {
			reqURL += "?" + params.Encode()
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL, nil)
		if err != nil {
			return nil, err
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-APIKEY", apiKey)

		resp, err := h.httpClient.Do(req)
		if err != nil {
			lastErr = err
			count := h.incrementFailureCount()

			if count >= constants.CircuitBreakerConfig.FailureThreshold {
				h.openCircuit()
				break
			}

			if attempt < maxAttempts-1 {
				delay := h.computeDelay(attempt)
				h.logger.Warn("Request failed, retrying",
					zap.Error(err),
					zap.Int("attempt", attempt+1),
					zap.Duration("delay", delay),
				)
				time.Sleep(delay)
				continue
			}
			break
		}

		// Read body and close immediately (not defer in loop!)
		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = err
			continue
		}

		// Handle rate limiting (429, 403)
		if resp.StatusCode == 429 || resp.StatusCode == 403 {
			h.logger.Warn("Rate limited, rotating key",
				zap.Int("status", resp.StatusCode),
				zap.Int("attempt", attempt+1),
			)

			if attempt < maxAttempts-1 {
				continue // Immediate retry with next key
			}

			return nil, errors.NewKeyRotationError("All API keys rate limited", resp.StatusCode, map[string]any{
				"url": reqURL,
			})
		}

		// Handle server errors (5xx)
		if resp.StatusCode >= 500 {
			count := h.incrementFailureCount()
			h.logger.Warn("Server error",
				zap.Int("status", resp.StatusCode),
				zap.Int("failure_count", count),
			)

			if count >= constants.CircuitBreakerConfig.FailureThreshold {
				h.openCircuit()
				break
			}

			if attempt < maxAttempts-1 {
				delay := h.computeDelay(attempt)
				time.Sleep(delay)
				continue
			}

			return nil, errors.NewAPIError(fmt.Sprintf("Server error: %d", resp.StatusCode), resp.StatusCode, nil)
		}

		// Handle client errors (4xx)
		if resp.StatusCode >= 400 {
			return nil, errors.NewAPIError(fmt.Sprintf("Client error: %d", resp.StatusCode), resp.StatusCode, map[string]any{
				"url":  reqURL,
				"body": string(body),
			})
		}

		// Success
		h.resetCircuit()
		return body, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, errors.NewAPIError("Holodex request failed after all retries", 502, nil)
}

// GetLiveStreams retrieves currently live streams
func (h *HolodexService) GetLiveStreams(ctx context.Context) ([]*domain.Stream, error) {
	cacheKey := "live_streams"

	// Check cache
	var cached []*domain.Stream
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil && cached != nil {
		return cached, nil
	}

	// Fetch from API
	params := url.Values{}
	params.Set("org", "Hololive")
	params.Set("status", "live")
	params.Set("type", "stream")

	body, err := h.doRequest(ctx, "GET", "/live", params)
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

	// Cache result
	_ = h.cache.Set(ctx, cacheKey, filtered, constants.CacheTTL.LiveStreams)

	return filtered, nil
}

// GetUpcomingStreams retrieves upcoming streams within specified hours
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

	body, err := h.doRequest(ctx, "GET", "/live", params)
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

// GetChannelSchedule retrieves schedule for a specific channel
func (h *HolodexService) GetChannelSchedule(ctx context.Context, channelID string, hours int, includeLive bool) ([]*domain.Stream, error) {
	cacheKey := fmt.Sprintf("channel_schedule_%s_%d_%t", channelID, hours, includeLive)

	var cached []*domain.Stream
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil && cached != nil {
		// Deep copy to prevent race conditions
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

		body, err := h.doRequest(ctx, "GET", "/live", params)
		if err != nil {
			h.logger.Error("Failed to get channel schedule",
				zap.String("channel_id", channelID),
				zap.String("status", string(status)),
				zap.Error(err),
			)

			// Try scraper fallback if available
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

	// Sort by start time
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

// SearchChannels searches for channels by name
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

	body, err := h.doRequest(ctx, "GET", "/channels", params)
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

	// Filter out HOLOSTARS
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

// GetChannel retrieves a specific channel by ID
func (h *HolodexService) GetChannel(ctx context.Context, channelID string) (*domain.Channel, error) {
	cacheKey := fmt.Sprintf("channel_%s", channelID)

	var cached domain.Channel
	if err := h.cache.Get(ctx, cacheKey, &cached); err == nil && cached.ID != "" {
		return &cached, nil
	}

	body, err := h.doRequest(ctx, "GET", "/channels/"+channelID, nil)
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

// mapStreamsResponse maps raw API streams to domain models
func (h *HolodexService) mapStreamsResponse(rawStreams []HolodexStreamRaw) []*domain.Stream {
	streams := make([]*domain.Stream, len(rawStreams))
	for i, raw := range rawStreams {
		streams[i] = h.mapStreamResponse(&raw)
	}
	return streams
}

// mapStreamResponse maps a single raw stream to domain model
func (h *HolodexService) mapStreamResponse(raw *HolodexStreamRaw) *domain.Stream {
	stream := &domain.Stream{
		ID:          raw.ID,
		Title:       raw.Title,
		Status:      raw.Status,
		Duration:    raw.Duration,
		Thumbnail:   raw.Thumbnail,
		Link:        raw.Link,
		TopicID:     raw.TopicID,
	}

	// ChannelID
	if raw.ChannelID != nil {
		stream.ChannelID = *raw.ChannelID
	} else if raw.Channel != nil {
		stream.ChannelID = raw.Channel.ID
	}

	// ChannelName
	if raw.Channel != nil {
		stream.ChannelName = raw.Channel.Name
	}

	// StartScheduled
	if raw.StartScheduled != nil && *raw.StartScheduled != "" {
		if t, err := time.Parse(time.RFC3339, *raw.StartScheduled); err == nil {
			stream.StartScheduled = &t
		}
	}

	// StartActual
	if raw.StartActual != nil && *raw.StartActual != "" {
		if t, err := time.Parse(time.RFC3339, *raw.StartActual); err == nil {
			stream.StartActual = &t
		}
	}

	// Channel
	if raw.Channel != nil {
		stream.Channel = h.mapChannelResponse(raw.Channel)
	}

	return stream
}

// mapChannelsResponse maps raw API channels to domain models
func (h *HolodexService) mapChannelsResponse(rawChannels []HolodexChannelRaw) []*domain.Channel {
	channels := make([]*domain.Channel, len(rawChannels))
	for i, raw := range rawChannels {
		channels[i] = h.mapChannelResponse(&raw)
	}
	return channels
}

// mapChannelResponse maps a single raw channel to domain model
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

// filterHololiveStreams filters out non-Hololive and HOLOSTARS streams
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

// filterUpcomingStreams filters out streams that have already started
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

// isHolostarsChannel checks if a channel belongs to HOLOSTARS
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


// shouldUseFallback determines if scraper fallback should be used
func (h *HolodexService) shouldUseFallback(err error) bool {
	if err == nil {
		return false
	}

	// Use fallback when circuit breaker is open
	if h.isCircuitOpen() {
		return true
	}

	// Use fallback for API errors
	if apiErr, ok := err.(*errors.APIError); ok {
		// Server errors (5xx)
		if apiErr.StatusCode >= 500 {
			return true
		}
		// Service unavailable
		if apiErr.StatusCode == 503 {
			return true
		}
	}

	// Use fallback for key rotation errors (all keys exhausted)
	if _, ok := err.(*errors.KeyRotationError); ok {
		return true
	}

	return false
}
