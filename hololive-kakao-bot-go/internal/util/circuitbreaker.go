package util

import (
	"sync"
	"time"

	"go.uber.org/zap"
)

type CircuitState string

const (
	CircuitStateClosed   CircuitState = "CLOSED"    // 정상 작동
	CircuitStateOpen     CircuitState = "OPEN"      // 서비스 차단
	CircuitStateHalfOpen CircuitState = "HALF_OPEN" // 복구 시도 중
)

func (s CircuitState) String() string {
	return string(s)
}

type HealthCheckFunction func() bool

type CircuitBreaker struct {
	state               CircuitState
	failureCount        int
	failureThreshold    int
	resetTimeout        time.Duration
	nextRetryTime       time.Time
	nextHealthCheckTime time.Time
	healthCheckInterval time.Duration
	isHealthChecking    bool
	healthCheckFn       HealthCheckFunction
	logger              *zap.Logger
	mu                  sync.RWMutex
}

func NewCircuitBreaker(
	failureThreshold int,
	resetTimeout time.Duration,
	healthCheckInterval time.Duration,
	healthCheckFn HealthCheckFunction,
	logger *zap.Logger,
) *CircuitBreaker {
	return &CircuitBreaker{
		state:               CircuitStateClosed,
		failureCount:        0,
		failureThreshold:    failureThreshold,
		resetTimeout:        resetTimeout,
		healthCheckInterval: healthCheckInterval,
		healthCheckFn:       healthCheckFn,
		logger:              logger,
	}
}

func (cb *CircuitBreaker) GetState() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitStateOpen {
		now := time.Now()

		if cb.healthCheckFn != nil && now.After(cb.nextHealthCheckTime) && !cb.isHealthChecking {
			go cb.tryHealthCheck()
		} else if cb.healthCheckFn == nil && now.After(cb.nextRetryTime) {
			cb.transitionTo(CircuitStateHalfOpen)
		}
	}

	return cb.state
}

func (cb *CircuitBreaker) CanExecute() bool {
	state := cb.GetState()
	return state != CircuitStateOpen
}

func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == CircuitStateHalfOpen {
		cb.logger.Info("Circuit Breaker: Service recovered, transitioning to CLOSED")
		cb.transitionTo(CircuitStateClosed)
		cb.failureCount = 0
	} else if cb.state == CircuitStateClosed && cb.failureCount > 0 {
		cb.logger.Debug("Circuit Breaker: Resetting failure count",
			zap.Int("was", cb.failureCount),
		)
		cb.failureCount = 0
	}
}

func (cb *CircuitBreaker) RecordFailure(customTimeout time.Duration) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++

	timeout := cb.resetTimeout
	if customTimeout > 0 {
		timeout = customTimeout
	}

	cb.logger.Warn("Circuit Breaker: Failure recorded",
		zap.Int("count", cb.failureCount),
		zap.Int("threshold", cb.failureThreshold),
		zap.Duration("timeout", timeout),
	)

	if cb.state == CircuitStateHalfOpen {
		cb.logger.Error("Circuit Breaker: Recovery failed, reopening circuit")
		cb.transitionTo(CircuitStateOpen)
		cb.nextRetryTime = time.Now().Add(timeout)

		if cb.healthCheckFn != nil {
			cb.nextHealthCheckTime = time.Now().Add(cb.healthCheckInterval)
		}
	} else if cb.failureCount >= cb.failureThreshold {
		cb.logger.Error("Circuit Breaker: Threshold reached, OPENING circuit",
			zap.Int("threshold", cb.failureThreshold),
		)
		cb.transitionTo(CircuitStateOpen)
		cb.nextRetryTime = time.Now().Add(timeout)

		if cb.healthCheckFn != nil {
			cb.nextHealthCheckTime = time.Now().Add(cb.healthCheckInterval)
		}
	}
}

func (cb *CircuitBreaker) tryHealthCheck() {
	cb.mu.Lock()
	if cb.healthCheckFn == nil || cb.isHealthChecking {
		cb.mu.Unlock()
		return
	}
	cb.isHealthChecking = true
	cb.mu.Unlock()

	cb.logger.Info("Circuit Breaker: Running health check...")

	isHealthy := cb.healthCheckFn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.isHealthChecking = false

	if isHealthy {
		cb.logger.Info("Circuit Breaker: Health check PASSED → transitioning to HALF_OPEN")
		cb.transitionTo(CircuitStateHalfOpen)
	} else {
		cb.logger.Warn("Circuit Breaker: Health check FAILED → delaying next check")
		cb.nextHealthCheckTime = time.Now().Add(cb.healthCheckInterval)
	}
}

func (cb *CircuitBreaker) transitionTo(newState CircuitState) {
	oldState := cb.state
	cb.state = newState

	nextRetry := "n/a"
	if newState == CircuitStateOpen {
		nextRetry = cb.nextRetryTime.Format(time.RFC3339)
	}

	cb.logger.Info("Circuit Breaker: State transition",
		zap.String("from", oldState.String()),
		zap.String("to", newState.String()),
		zap.Int("failure_count", cb.failureCount),
		zap.String("next_retry", nextRetry),
	)
}

func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.logger.Info("Circuit Breaker: Manual reset")
	cb.state = CircuitStateClosed
	cb.failureCount = 0
	cb.nextRetryTime = time.Time{}
}

func (cb *CircuitBreaker) GetStatus() CircuitBreakerStatus {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	status := CircuitBreakerStatus{
		State:        cb.state, // Direct access (already locked)
		FailureCount: cb.failureCount,
	}

	if cb.state == CircuitStateOpen {
		status.NextRetryTime = &cb.nextRetryTime
	}

	return status
}

type CircuitBreakerStatus struct {
	State         CircuitState
	FailureCount  int
	NextRetryTime *time.Time
}
