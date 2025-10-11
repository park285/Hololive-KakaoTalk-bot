package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

type ModelManager struct {
	gemini         *GeminiProvider
	openai         *OpenAIProvider
	primary        JSONProvider
	fallback       JSONProvider
	logger         *zap.Logger
	enableFallback bool
	circuitBreaker *util.CircuitBreaker
}

type ModelManagerConfig struct {
	GeminiAPIKey       string
	OpenAIAPIKey       string
	DefaultGeminiModel string
	DefaultOpenAIModel string
	EnableFallback     bool
}

func NewModelManager(ctx context.Context, cfg ModelManagerConfig, logger *zap.Logger) (*ModelManager, error) {
	geminiClient, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  cfg.GeminiAPIKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	defaultGemini := cfg.DefaultGeminiModel
	if defaultGemini == "" {
		defaultGemini = "gemini-2.5-flash"
	}

	defaultOpenAI := cfg.DefaultOpenAIModel
	if defaultOpenAI == "" {
		defaultOpenAI = "gpt-5-mini"
	}

	geminiProvider := NewGeminiProvider(geminiClient, defaultGemini, logger)

	openaiProvider := NewOpenAIProvider(cfg.OpenAIAPIKey, defaultOpenAI, logger)
	if openaiProvider != nil {
		logger.Info("OpenAI fallback enabled", zap.String("model", defaultOpenAI))
	} else {
		logger.Info("OpenAI fallback disabled (no API key)")
	}

	mm := &ModelManager{
		gemini:  geminiProvider,
		openai:  openaiProvider,
		primary: geminiProvider,
		logger:  logger,
	}
	mm.enableFallback = cfg.EnableFallback && openaiProvider != nil
	if mm.enableFallback {
		mm.fallback = openaiProvider
	}

	mm.circuitBreaker = util.NewCircuitBreaker(
		constants.CircuitBreakerConfig.FailureThreshold,
		constants.CircuitBreakerConfig.ResetTimeout,
		constants.CircuitBreakerConfig.HealthCheckInterval,
		mm.healthCheckPing,
		logger,
	)

	return mm, nil
}

func (mm *ModelManager) GetGeminiClient() *genai.Client {
	if mm.gemini == nil {
		return nil
	}
	return mm.gemini.Client()
}

func (mm *ModelManager) GetDefaultGeminiModel() string {
	if mm.gemini == nil {
		return ""
	}
	return mm.gemini.DefaultModel()
}

func (mm *ModelManager) GenerateJSON(ctx context.Context, prompt string, preset ModelPreset, dest any, opts *GenerateOptions) (*GenerateMetadata, error) {
	if !mm.circuitBreaker.CanExecute() {
		status := mm.circuitBreaker.GetStatus()
		nextRetry := "알 수 없음"
		if status.NextRetryTime != nil {
			nextRetry = util.FormatKST(*status.NextRetryTime, "15:04")
		}

		mm.logger.Error("AI service unavailable (Circuit OPEN)",
			zap.String("state", status.State.String()),
			zap.Int("failure_count", status.FailureCount),
			zap.String("next_retry", nextRetry),
		)

		return nil, fmt.Errorf("⚠️ 외부 AI 서비스 장애 감지\nGoogle/OpenAI API에 일시적인 문제가 발생했습니다.\n\n🔄 자동 복구 대기 중 (%s 재확인 → 복구 시 자동 재개)", nextRetry)
	}

	var options GenerateOptions
	if opts != nil {
		options = *opts
	}
	options.JSONMode = true

	primaryResult, primaryErr := mm.invokeProvider(ctx, mm.primary, prompt, preset, &options)
	if primaryErr == nil {
		mm.circuitBreaker.RecordSuccess()
		metadata := &GenerateMetadata{
			Provider: mm.primary.Name(),
			Model:    primaryResult.Model,
		}
		return mm.decodeJSON(primaryResult.Text, metadata, dest)
	}

	if mm.enableFallback && mm.fallback != nil {
		fallbackResult, fallbackErr := mm.invokeProvider(ctx, mm.fallback, prompt, preset, &options)
		if fallbackErr == nil {
			mm.circuitBreaker.RecordSuccess()
			metadata := &GenerateMetadata{
				Provider:     mm.fallback.Name(),
				Model:        fallbackResult.Model,
				UsedFallback: true,
			}
			return mm.decodeJSON(fallbackResult.Text, metadata, dest)
		}

		mm.recordFailure(primaryErr)
		mm.recordFailure(fallbackErr)

		if mm.isServiceFailure(primaryErr) || mm.isServiceFailure(fallbackErr) {
			return nil, fmt.Errorf("AI 서비스에 일시적인 문제가 발생했습니다. 잠시 후 다시 시도해주세요")
		}

		return nil, fallbackErr
	}

	mm.recordFailure(primaryErr)

	if mm.isServiceFailure(primaryErr) {
		return nil, fmt.Errorf("AI 서비스에 일시적인 문제가 발생했습니다. 잠시 후 다시 시도해주세요")
	}

	return nil, primaryErr
}

func (mm *ModelManager) invokeProvider(ctx context.Context, provider JSONProvider, prompt string, preset ModelPreset, opts *GenerateOptions) (ProviderResult, error) {
	if provider == nil {
		return ProviderResult{}, fmt.Errorf("model provider is not configured")
	}
	return provider.Generate(ctx, prompt, preset, opts)
}

func (mm *ModelManager) decodeJSON(text string, metadata *GenerateMetadata, dest any) (*GenerateMetadata, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil, fmt.Errorf("%s API returned empty response", metadata.Provider)
	}

	cleaned := trimmed
	if strings.HasPrefix(cleaned, "```json") {
		cleaned = strings.TrimPrefix(cleaned, "```json")
		cleaned = strings.TrimSpace(cleaned)
	} else if strings.HasPrefix(cleaned, "```") {
		cleaned = strings.TrimPrefix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}
	if strings.HasSuffix(cleaned, "```") {
		cleaned = strings.TrimSuffix(cleaned, "```")
		cleaned = strings.TrimSpace(cleaned)
	}

	if err := json.Unmarshal([]byte(cleaned), dest); err != nil {
		previewLen := util.Min(len(cleaned), 200)
		mm.logger.Error("Failed to unmarshal JSON response",
			zap.String("provider", metadata.Provider),
			zap.Error(err),
			zap.String("response_preview", cleaned[:previewLen]),
		)
		return nil, fmt.Errorf("invalid JSON from %s: %w", metadata.Provider, err)
	}

	return metadata, nil
}

func (mm *ModelManager) recordFailure(err error) {
	if err == nil {
		return
	}

	if !mm.isServiceFailure(err) {
		return
	}

	timeout := constants.CircuitBreakerConfig.ResetTimeout
	if mm.isRateLimitError(err) {
		timeout = constants.CircuitBreakerConfig.RateLimitTimeout
	}

	mm.circuitBreaker.RecordFailure(timeout)
}

func (mm *ModelManager) healthCheckPing() bool {
	mm.logger.Info("Health Check: Testing AI services...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	geminiOK := false
	if mm.gemini != nil {
		geminiOK = mm.gemini.Ping(ctx)
	}

	openaiOK := false
	if mm.enableFallback && mm.openai != nil {
		openaiOK = mm.openai.Ping(ctx)
	}

	isHealthy := geminiOK || openaiOK

	mm.logger.Info("Health Check: Result",
		zap.Bool("gemini", geminiOK),
		zap.Bool("openai", openaiOK),
		zap.Bool("healthy", isHealthy),
	)

	return isHealthy
}

func (mm *ModelManager) isServiceFailure(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	if strings.Contains(msg, "timeout") || strings.Contains(msg, "ETIMEDOUT") {
		return true
	}

	if mm.isRateLimitError(err) {
		return true
	}

	statusRegex := regexp.MustCompile(`\b(5\d{2})\b`)
	if statusRegex.MatchString(msg) {
		return true
	}

	geminiCodeRegex := regexp.MustCompile(`"code":(\d{3})`)
	if matches := geminiCodeRegex.FindStringSubmatch(msg); len(matches) > 1 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return code >= 500 && code < 600
		}
	}

	openaiCodeRegex := regexp.MustCompile(`^(\d{3})\s`)
	if matches := openaiCodeRegex.FindStringSubmatch(msg); len(matches) > 1 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return code >= 500 && code < 600
		}
	}

	return false
}

func (mm *ModelManager) isRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()

	if strings.Contains(msg, "429") || strings.Contains(msg, "Rate limit") || strings.Contains(msg, "quota") {
		return true
	}

	geminiCodeRegex := regexp.MustCompile(`"code":(\d{3})`)
	if matches := geminiCodeRegex.FindStringSubmatch(msg); len(matches) > 1 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return code == 429
		}
	}

	openaiCodeRegex := regexp.MustCompile(`^(\d{3})\s`)
	if matches := openaiCodeRegex.FindStringSubmatch(msg); len(matches) > 1 {
		if code, err := strconv.Atoi(matches[1]); err == nil {
			return code == 429
		}
	}

	return false
}

func (mm *ModelManager) GetCircuitStatus() util.CircuitBreakerStatus {
	return mm.circuitBreaker.GetStatus()
}

func (mm *ModelManager) ResetCircuit() {
	mm.circuitBreaker.Reset()
}
