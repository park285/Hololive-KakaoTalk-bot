package service

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
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

type ModelManager struct {
	geminiClient       *genai.Client
	openaiClient       *openai.Client
	logger             *zap.Logger
	defaultGeminiModel string
	defaultOpenAIModel string
	enableFallback     bool
	circuitBreaker     *util.CircuitBreaker
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

	mm := &ModelManager{
		geminiClient:       geminiClient,
		logger:             logger,
		defaultGeminiModel: defaultGemini,
		defaultOpenAIModel: defaultOpenAI,
		enableFallback:     cfg.EnableFallback && cfg.OpenAIAPIKey != "",
	}

	if cfg.OpenAIAPIKey != "" {
		client := openai.NewClient(option.WithAPIKey(cfg.OpenAIAPIKey))
		mm.openaiClient = &client
		logger.Info("OpenAI fallback enabled", zap.String("model", defaultOpenAI))
	} else {
		logger.Info("OpenAI fallback disabled (no API key)")
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
	return mm.geminiClient
}

func (mm *ModelManager) GetDefaultGeminiModel() string {
	return mm.defaultGeminiModel
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

	var text string
	var metadata *GenerateMetadata

	if opts == nil {
		opts = &GenerateOptions{}
	}
	opts.JSONMode = true

	geminiText, geminiErr := mm.generateWithGemini(ctx, prompt, preset, opts)
	if geminiErr == nil {
		mm.circuitBreaker.RecordSuccess()
		text = geminiText
		metadata = &GenerateMetadata{
			Provider:     "Gemini",
			Model:        mm.getGeminiModel(opts),
			UsedFallback: false,
		}
	} else {
		if mm.enableFallback && mm.openaiClient != nil {
			openaiText, openaiErr := mm.generateWithOpenAI(ctx, prompt, preset, opts)
			if openaiErr == nil {
				mm.circuitBreaker.RecordSuccess()
				text = openaiText
				metadata = &GenerateMetadata{
					Provider:     "OpenAI",
					Model:        mm.getOpenAIModel(opts),
					UsedFallback: true,
				}
			} else {
				if mm.isServiceFailure(geminiErr) || mm.isServiceFailure(openaiErr) {
					timeout := constants.CircuitBreakerConfig.ResetTimeout
					if mm.isRateLimitError(geminiErr) || mm.isRateLimitError(openaiErr) {
						timeout = constants.CircuitBreakerConfig.RateLimitTimeout
					}
					mm.circuitBreaker.RecordFailure(timeout)
				}
				return nil, fmt.Errorf("AI 서비스에 일시적인 문제가 발생했습니다. 잠시 후 다시 시도해주세요")
			}
		} else {
			if mm.isServiceFailure(geminiErr) {
				timeout := constants.CircuitBreakerConfig.ResetTimeout
				if mm.isRateLimitError(geminiErr) {
					timeout = constants.CircuitBreakerConfig.RateLimitTimeout
				}
				mm.circuitBreaker.RecordFailure(timeout)
			}
			return nil, geminiErr
		}
	}

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

func (mm *ModelManager) getGeminiModel(opts *GenerateOptions) string {
	if opts != nil && opts.Model != "" {
		return opts.Model
	}
	return mm.defaultGeminiModel
}

func (mm *ModelManager) getOpenAIModel(opts *GenerateOptions) string {
	if opts != nil && opts.Model != "" {
		return opts.Model
	}
	return mm.defaultOpenAIModel
}

func (mm *ModelManager) generateWithGemini(ctx context.Context, prompt string, preset ModelPreset, opts *GenerateOptions) (string, error) {
	modelName := mm.getGeminiModel(opts)
	config := GetPresetConfig(preset)

	if opts != nil && opts.Overrides != nil {
		if opts.Overrides.Temperature > 0 {
			config.Temperature = opts.Overrides.Temperature
		}
		if opts.Overrides.TopP > 0 {
			config.TopP = opts.Overrides.TopP
		}
		if opts.Overrides.TopK > 0 {
			config.TopK = opts.Overrides.TopK
		}
		if opts.Overrides.MaxOutputTokens > 0 {
			config.MaxOutputTokens = opts.Overrides.MaxOutputTokens
		}
	}

	if opts != nil && opts.JSONMode {
		config.ResponseMimeType = "application/json"
	}

	mm.logger.Debug("Generating with Gemini",
		zap.String("model", modelName),
		zap.String("preset", string(preset)),
		zap.Bool("json_mode", opts != nil && opts.JSONMode),
	)

	topK := float32(config.TopK)
	maxTokens := int32(config.MaxOutputTokens)

	genConfig := &genai.GenerateContentConfig{
		Temperature:      &config.Temperature,
		TopP:             &config.TopP,
		TopK:             &topK,
		MaxOutputTokens:  maxTokens,
		ResponseMIMEType: config.ResponseMimeType,
	}

	if opts != nil && opts.CachedContent != "" {
		genConfig.CachedContent = opts.CachedContent
		mm.logger.Debug("Using cached content", zap.String("cache_name", opts.CachedContent))
	}

	resp, err := mm.geminiClient.Models.GenerateContent(ctx, modelName, []*genai.Content{
		{
			Parts: []*genai.Part{
				{Text: prompt},
			},
		},
	}, genConfig)

	if err != nil {
		mm.logger.Error("Gemini generation failed", zap.Error(err))
		return "", err
	}

	text := extractTextFromGeminiResponse(resp)
	if text == "" {
		return "", fmt.Errorf("empty response from Gemini")
	}

	mm.logger.Debug("Gemini response received", zap.Int("length", len(text)))
	return text, nil
}

func (mm *ModelManager) generateWithOpenAI(ctx context.Context, prompt string, preset ModelPreset, opts *GenerateOptions) (string, error) {
	if mm.openaiClient == nil {
		return "", fmt.Errorf("OpenAI client not initialized")
	}

	modelName := mm.getOpenAIModel(opts)
	config := GetOpenAIPresetConfig(preset)

	mm.logger.Info("Fallback: Generating with OpenAI",
		zap.String("model", modelName),
		zap.String("preset", string(preset)),
	)

	var model openai.ChatModel
	switch modelName {
	case "gpt-5-mini":
		model = openai.ChatModelGPT5Mini
	case "gpt-5":
		model = openai.ChatModelGPT5
	case "gpt-5-nano":
		model = openai.ChatModelGPT5Nano
	case "gpt-4.1":
		model = openai.ChatModelGPT4_1
	case "gpt-4.1-mini":
		model = openai.ChatModelGPT4_1Mini
	case "gpt-4.1-nano":
		model = openai.ChatModelGPT4_1Nano
	case "gpt-4o":
		model = openai.ChatModelGPT4o
	case "gpt-4o-mini":
		model = openai.ChatModelGPT4oMini
	case "gpt-4-turbo":
		model = openai.ChatModelGPT4Turbo
	default:
		model = openai.ChatModelGPT4_1
	}

	messages := []openai.ChatCompletionMessageParamUnion{
		openai.UserMessage(prompt),
	}

	if opts != nil && opts.JSONMode {
		messages = []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage("You must respond with valid JSON only. Do not include any text outside the JSON object."),
			openai.UserMessage(prompt),
		}
	}

	isGPT5 := modelName == "gpt-5" || modelName == "gpt-5-mini" || modelName == "gpt-5-nano"
	
	params := openai.ChatCompletionNewParams{
		Model:               model,
		Messages:            messages,
		MaxCompletionTokens: openai.Int(int64(config.MaxTokens)),
	}
	
	if !isGPT5 {
		params.Temperature = openai.Float(float64(config.Temperature))
		params.TopP = openai.Float(float64(config.TopP))
	}

	resp, err := mm.openaiClient.Chat.Completions.New(ctx, params)
	if err != nil {
		mm.logger.Error("OpenAI generation failed", zap.Error(err))
		return "", err
	}

	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("no choices in OpenAI response")
	}

	text := resp.Choices[0].Message.Content

	cachedTokens := int64(0)
	if resp.Usage.PromptTokensDetails.CachedTokens > 0 {
		cachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
	}

	mm.logger.Info("OpenAI response received",
		zap.Int("length", len(text)),
		zap.Int64("prompt_tokens", resp.Usage.PromptTokens),
		zap.Int64("completion_tokens", resp.Usage.CompletionTokens),
		zap.Int64("cached_tokens", cachedTokens),
		zap.Bool("cache_hit", cachedTokens > 0),
	)

	return text, nil
}

func (mm *ModelManager) healthCheckPing() bool {
	mm.logger.Info("Health Check: Testing AI services...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	geminiOK := mm.pingGemini(ctx)
	openaiOK := false

	if mm.enableFallback && mm.openaiClient != nil {
		openaiOK = mm.pingOpenAI(ctx)
	}

	isHealthy := geminiOK || openaiOK

	mm.logger.Info("Health Check: Result",
		zap.Bool("gemini", geminiOK),
		zap.Bool("openai", openaiOK),
		zap.Bool("healthy", isHealthy),
	)

	return isHealthy
}

func (mm *ModelManager) pingGemini(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	mm.logger.Debug("Pinging Gemini API...")

	temp := float32(0)
	topP := float32(1)
	topK := float32(1)

	config := &genai.GenerateContentConfig{
		Temperature:     &temp,
		TopP:            &topP,
		TopK:            &topK,
		MaxOutputTokens: 10,
	}

	resp, err := mm.geminiClient.Models.GenerateContent(ctx, mm.defaultGeminiModel, []*genai.Content{
		{Parts: []*genai.Part{{Text: "ping"}}},
	}, config)

	if err != nil {
		mm.logger.Debug("Gemini ping failed", zap.Error(err))
		return false
	}

	return extractTextFromGeminiResponse(resp) != ""
}

func (mm *ModelManager) pingOpenAI(ctx context.Context) bool {
	if mm.openaiClient == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	mm.logger.Debug("Pinging OpenAI API...")

	resp, err := mm.openaiClient.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT4o,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("ping"),
		},
		MaxTokens:   openai.Int(10),
		Temperature: openai.Float(0),
	})

	if err != nil {
		mm.logger.Debug("OpenAI ping failed", zap.Error(err))
		return false
	}

	return len(resp.Choices) > 0
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

func extractTextFromGeminiResponse(resp *genai.GenerateContentResponse) string {
	if resp == nil || len(resp.Candidates) == 0 {
		return ""
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return ""
	}

	var texts []string
	for _, part := range candidate.Content.Parts {
		if part.Text != "" {
			texts = append(texts, part.Text)
		}
	}

	return strings.Join(texts, "")
}

func (mm *ModelManager) GetCircuitStatus() util.CircuitBreakerStatus {
	return mm.circuitBreaker.GetStatus()
}

func (mm *ModelManager) ResetCircuit() {
	mm.circuitBreaker.Reset()
}
