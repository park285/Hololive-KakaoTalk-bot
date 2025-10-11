package service

import (
	"context"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"github.com/kapu/hololive-kakao-bot-go/pkg/errors"
	"go.uber.org/zap"
)

type HolodexRequester interface {
	DoRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error)
	IsCircuitOpen() bool
}

type HolodexAPIClient struct {
	httpClient       *http.Client
	apiKeys          []string
	currentKeyIndex  int
	keyMu            sync.Mutex
	logger           *zap.Logger
	failureCount     int
	failureMu        sync.Mutex
	circuitOpenUntil *time.Time
	circuitMu        sync.RWMutex
}

func NewHolodexAPIClient(httpClient *http.Client, apiKeys []string, logger *zap.Logger) *HolodexAPIClient {
	return &HolodexAPIClient{
		httpClient: httpClient,
		apiKeys:    apiKeys,
		logger:     logger,
	}
}

func (c *HolodexAPIClient) DoRequest(ctx context.Context, method, path string, params url.Values) ([]byte, error) {
	if c.IsCircuitOpen() {
		c.circuitMu.RLock()
		var remainingMs int64
		if c.circuitOpenUntil != nil {
			remainingMs = time.Until(*c.circuitOpenUntil).Milliseconds()
		}
		c.circuitMu.RUnlock()

		c.logger.Warn("Circuit breaker is open", zap.Int64("retry_after_ms", remainingMs))
		return nil, errors.NewAPIError("Circuit breaker open", 503, map[string]any{
			"retry_after_ms": remainingMs,
		})
	}

	maxAttempts := util.Min(len(c.apiKeys)*2, 10)
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		apiKey := c.getNextAPIKey()

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

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			count := c.incrementFailureCount()

			if count >= constants.CircuitBreakerConfig.FailureThreshold {
				c.openCircuit()
				break
			}

			if attempt < maxAttempts-1 {
				delay := c.computeDelay(attempt)
				c.logger.Warn("Request failed, retrying",
					zap.Error(err),
					zap.Int("attempt", attempt+1),
					zap.Duration("delay", delay),
				)
				time.Sleep(delay)
				continue
			}
			break
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()

		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode == 429 || resp.StatusCode == 403 {
			c.logger.Warn("Rate limited, rotating key",
				zap.Int("status", resp.StatusCode),
				zap.Int("attempt", attempt+1),
			)

			if attempt < maxAttempts-1 {
				continue
			}

			return nil, errors.NewKeyRotationError("All API keys rate limited", resp.StatusCode, map[string]any{
				"url": reqURL,
			})
		}

		if resp.StatusCode >= 500 {
			count := c.incrementFailureCount()
			c.logger.Warn("Server error",
				zap.Int("status", resp.StatusCode),
				zap.Int("failure_count", count),
			)

			if count >= constants.CircuitBreakerConfig.FailureThreshold {
				c.openCircuit()
				break
			}

			if attempt < maxAttempts-1 {
				delay := c.computeDelay(attempt)
				time.Sleep(delay)
				continue
			}

			return nil, errors.NewAPIError(fmt.Sprintf("Server error: %d", resp.StatusCode), resp.StatusCode, nil)
		}

		if resp.StatusCode >= 400 {
			return nil, errors.NewAPIError(fmt.Sprintf("Client error: %d", resp.StatusCode), resp.StatusCode, map[string]any{
				"url":  reqURL,
				"body": string(body),
			})
		}

		c.resetCircuit()
		return body, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, fmt.Errorf("holodex request failed")
}

func (c *HolodexAPIClient) IsCircuitOpen() bool {
	c.circuitMu.RLock()
	defer c.circuitMu.RUnlock()

	if c.circuitOpenUntil == nil {
		return false
	}

	if time.Now().After(*c.circuitOpenUntil) {
		return false
	}

	return true
}

func (c *HolodexAPIClient) getNextAPIKey() string {
	c.keyMu.Lock()
	defer c.keyMu.Unlock()

	key := c.apiKeys[c.currentKeyIndex]
	c.currentKeyIndex = (c.currentKeyIndex + 1) % len(c.apiKeys)
	return key
}

func (c *HolodexAPIClient) openCircuit() {
	c.circuitMu.Lock()
	defer c.circuitMu.Unlock()

	resetTime := time.Now().Add(constants.CircuitBreakerConfig.ResetTimeout)
	c.circuitOpenUntil = &resetTime
	c.failureCount = 0

	c.logger.Error("Holodex circuit breaker opened",
		zap.Duration("reset_timeout", constants.CircuitBreakerConfig.ResetTimeout),
	)
}

func (c *HolodexAPIClient) resetCircuit() {
	c.circuitMu.Lock()
	defer c.circuitMu.Unlock()

	c.failureCount = 0
	c.circuitOpenUntil = nil
}

func (c *HolodexAPIClient) incrementFailureCount() int {
	c.failureMu.Lock()
	defer c.failureMu.Unlock()

	c.failureCount++
	return c.failureCount
}

func (c *HolodexAPIClient) computeDelay(attempt int) time.Duration {
	base := constants.RetryConfig.BaseDelay * time.Duration(math.Pow(2, float64(attempt)))
	jitter := time.Duration(rand.Float64() * float64(constants.RetryConfig.Jitter))
	return base + jitter
}
