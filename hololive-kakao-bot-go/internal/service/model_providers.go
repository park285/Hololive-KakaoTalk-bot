package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

type JSONProvider interface {
	Name() string
	Generate(ctx context.Context, prompt string, preset ModelPreset, opts *GenerateOptions) (ProviderResult, error)
	Ping(ctx context.Context) bool
}

type ProviderResult struct {
	Text  string
	Model string
}

// GeminiProvider wraps the Gemini client with preset-aware generation logic.
type GeminiProvider struct {
	client       *genai.Client
	defaultModel string
	logger       *zap.Logger
}

func NewGeminiProvider(client *genai.Client, defaultModel string, logger *zap.Logger) *GeminiProvider {
	return &GeminiProvider{
		client:       client,
		defaultModel: defaultModel,
		logger:       logger,
	}
}

func (g *GeminiProvider) Name() string {
	return "Gemini"
}

func (g *GeminiProvider) DefaultModel() string {
	return g.defaultModel
}

func (g *GeminiProvider) Client() *genai.Client {
	return g.client
}

func (g *GeminiProvider) Generate(ctx context.Context, prompt string, preset ModelPreset, opts *GenerateOptions) (ProviderResult, error) {
	if g.client == nil {
		return ProviderResult{}, fmt.Errorf("gemini client not initialized")
	}

	modelName := g.getModel(opts)
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

	g.logger.Debug("Generating with Gemini",
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
		g.logger.Debug("Using cached content", zap.String("cache_name", opts.CachedContent))
	}

	resp, err := g.client.Models.GenerateContent(ctx, modelName, []*genai.Content{
		{
			Parts: []*genai.Part{
				{Text: prompt},
			},
		},
	}, genConfig)

	if err != nil {
		g.logger.Error("Gemini generation failed", zap.Error(err))
		return ProviderResult{}, err
	}

	text := extractTextFromGeminiResponse(resp)
	if text == "" {
		return ProviderResult{}, fmt.Errorf("empty response from Gemini")
	}

	g.logger.Debug("Gemini response received", zap.Int("length", len(text)))
	return ProviderResult{Text: text, Model: modelName}, nil
}

func (g *GeminiProvider) Ping(ctx context.Context) bool {
	if g.client == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	g.logger.Debug("Pinging Gemini API...")

	temp := float32(0)
	topP := float32(1)
	topK := float32(1)

	config := &genai.GenerateContentConfig{
		Temperature:     &temp,
		TopP:            &topP,
		TopK:            &topK,
		MaxOutputTokens: 10,
	}

	resp, err := g.client.Models.GenerateContent(ctx, g.defaultModel, []*genai.Content{
		{Parts: []*genai.Part{{Text: "ping"}}},
	}, config)

	if err != nil {
		g.logger.Debug("Gemini ping failed", zap.Error(err))
		return false
	}

	return extractTextFromGeminiResponse(resp) != ""
}

func (g *GeminiProvider) getModel(opts *GenerateOptions) string {
	if opts != nil && opts.Model != "" {
		return opts.Model
	}
	return g.defaultModel
}

// OpenAIProvider wraps the OpenAI chat completion client.
type OpenAIProvider struct {
	client       *openai.Client
	defaultModel string
	logger       *zap.Logger
}

func NewOpenAIProvider(apiKey string, defaultModel string, logger *zap.Logger) *OpenAIProvider {
	if apiKey == "" {
		return nil
	}
	client := openai.NewClient(option.WithAPIKey(apiKey))
	return &OpenAIProvider{
		client:       &client,
		defaultModel: defaultModel,
		logger:       logger,
	}
}

func (o *OpenAIProvider) Name() string {
	return "OpenAI"
}

func (o *OpenAIProvider) Generate(ctx context.Context, prompt string, preset ModelPreset, opts *GenerateOptions) (ProviderResult, error) {
	if o.client == nil {
		return ProviderResult{}, fmt.Errorf("OpenAI client not initialized")
	}

	modelName := o.getModel(opts)
	config := GetOpenAIPresetConfig(preset)

	o.logger.Info("Fallback: Generating with OpenAI",
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

	resp, err := o.client.Chat.Completions.New(ctx, params)
	if err != nil {
		o.logger.Error("OpenAI generation failed", zap.Error(err))
		return ProviderResult{}, err
	}

	if len(resp.Choices) == 0 {
		return ProviderResult{}, fmt.Errorf("no choices in OpenAI response")
	}

	text := resp.Choices[0].Message.Content

	cachedTokens := int64(0)
	if resp.Usage.PromptTokensDetails.CachedTokens > 0 {
		cachedTokens = resp.Usage.PromptTokensDetails.CachedTokens
	}

	o.logger.Info("OpenAI response received",
		zap.Int("length", len(text)),
		zap.Int64("prompt_tokens", resp.Usage.PromptTokens),
		zap.Int64("completion_tokens", resp.Usage.CompletionTokens),
		zap.Int64("cached_tokens", cachedTokens),
		zap.Bool("cache_hit", cachedTokens > 0),
	)

	return ProviderResult{Text: text, Model: modelName}, nil
}

func (o *OpenAIProvider) Ping(ctx context.Context) bool {
	if o.client == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	o.logger.Debug("Pinging OpenAI API...")

	resp, err := o.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModelGPT4o,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("ping"),
		},
		MaxTokens:   openai.Int(10),
		Temperature: openai.Float(0),
	})

	if err != nil {
		o.logger.Debug("OpenAI ping failed", zap.Error(err))
		return false
	}

	return len(resp.Choices) > 0
}

func (o *OpenAIProvider) getModel(opts *GenerateOptions) string {
	if opts != nil && opts.Model != "" {
		return opts.Model
	}
	return o.defaultModel
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
