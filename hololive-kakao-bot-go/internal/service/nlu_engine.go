package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/prompt"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"go.uber.org/zap"
)

type ModelInvoker interface {
	GenerateJSON(ctx context.Context, prompt string, preset ModelPreset, dest any, opts *GenerateOptions) (*GenerateMetadata, error)
}

type ParseEngine struct {
	invoker           ModelInvoker
	promptBuilder     *prompt.PromptBuilder
	directoryBuilder  *MemberDirectoryBuilder
	cache             *ParseCache
	logger            *zap.Logger
	parsePreset       ModelPreset
	selectorPreset    ModelPreset
	selectorMaxTokens int
}

func NewParseEngine(
	invoker ModelInvoker,
	promptBuilder *prompt.PromptBuilder,
	directoryBuilder *MemberDirectoryBuilder,
	cache *ParseCache,
	logger *zap.Logger,
) *ParseEngine {
	return &ParseEngine{
		invoker:           invoker,
		promptBuilder:     promptBuilder,
		directoryBuilder:  directoryBuilder,
		cache:             cache,
		logger:            logger,
		parsePreset:       PresetPrecise,
		selectorPreset:    PresetPrecise,
		selectorMaxTokens: 256,
	}
}

func (e *ParseEngine) Parse(ctx context.Context, query string, members domain.MemberDataProvider) (*domain.ParseResults, *GenerateMetadata, error) {
	normalizedQuery := util.Normalize(query)
	cacheKey := fmt.Sprintf("parse:%s", normalizedQuery)

	if entry, ok := e.cache.Get(cacheKey); ok {
		return entry.Result, entry.Metadata, nil
	}

	sanitized := sanitizeInput(query)
	if sanitized == "" {
		e.logger.Warn("Empty query after sanitization")
		result := unknownParseResult("입력된 질문이 비어 있습니다.")
		metadata := &GenerateMetadata{
			Provider: "None",
			Model:    "n/a",
		}
		e.cache.Set(cacheKey, result, metadata)
		return result, metadata, nil
	}

	normalized := sanitized
	if withoutSuffix := util.NormalizeSuffix(sanitized); withoutSuffix != sanitized {
		normalized = withoutSuffix
		e.logger.Info("Normalized query suffix",
			zap.String("original", sanitized),
			zap.String("normalized", normalized),
		)
	}

	promptText, err := e.buildParserPrompt(normalized, members)
	if err != nil {
		return nil, nil, err
	}

	var rawResult any
	metadata, err := e.invoker.GenerateJSON(ctx, promptText, e.parsePreset, &rawResult, nil)
	if err != nil {
		if strings.Contains(err.Error(), "외부 AI 서비스 장애") {
			return nil, nil, err
		}

		e.logger.Error("Failed to parse natural language", zap.Error(err))
		result := unknownParseResult("자연어 처리 중 오류가 발생했습니다.")
		metadata = &GenerateMetadata{
			Provider: "Unknown",
			Model:    "error",
		}
		e.cache.Set(cacheKey, result, metadata)
		return result, metadata, nil
	}

	parseResults, err := parseAIResponse(rawResult)
	if err != nil {
		e.logger.Error("Failed to parse AI response", zap.Error(err))
		result := unknownParseResult("AI 응답 형식이 올바르지 않습니다.")
		e.cache.Set(cacheKey, result, metadata)
		return result, metadata, nil
	}

	e.cache.Set(cacheKey, parseResults, metadata)
	return parseResults, metadata, nil
}

func (e *ParseEngine) buildParserPrompt(query string, members domain.MemberDataProvider) (string, error) {
	memberList := e.directoryBuilder.MemberListWithAliases(members)
	data := prompt.ParserPromptData{
		MemberCount:       len(members.GetAllMembers()),
		MemberListWithIDs: memberList,
		UserQuery:         query,
	}

	text, err := e.promptBuilder.Render(prompt.TemplateParserPrompt, data)
	if err != nil {
		e.logger.Error("Failed to render parser prompt template, using fallback", zap.Error(err))
		return prompt.FallbackParserPrompt(data), nil
	}

	return text, nil
}

func (e *ParseEngine) SelectBestChannel(ctx context.Context, query string, candidates []*domain.Channel) (*domain.Channel, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	sanitized := sanitizeInput(query)
	if sanitized == "" {
		e.logger.Warn("Channel selection skipped: empty query")
		return nil, nil
	}

	data := prompt.ChannelSelectorData{
		UserQuery:         sanitized,
		CandidateChannels: make([]prompt.ChannelCandidate, len(candidates)),
	}

	for i, ch := range candidates {
		englishName := "N/A"
		if ch.EnglishName != nil {
			englishName = *ch.EnglishName
		}
		data.CandidateChannels[i] = prompt.ChannelCandidate{
			Index:       i,
			Name:        ch.Name,
			EnglishName: englishName,
			ID:          ch.ID,
		}
	}

	promptText, err := e.promptBuilder.Render(prompt.TemplateChannelSelector, data)
	if err != nil {
		e.logger.Error("Failed to render channel selector template, using fallback", zap.Error(err))
		promptText = prompt.FallbackChannelSelector(data)
	}

	var selection domain.ChannelSelection
	opts := &GenerateOptions{
		Overrides: &ModelConfig{
			MaxOutputTokens: e.selectorMaxTokens,
		},
	}

	metadata, err := e.invoker.GenerateJSON(ctx, promptText, e.selectorPreset, &selection, opts)
	if err != nil {
		e.logger.Error("Failed to select channel", zap.Error(err))
		return nil, err
	}

	e.logger.Info("Channel selection",
		zap.String("query", query),
		zap.Int("selected_index", selection.SelectedIndex),
		zap.Float64("confidence", selection.Confidence),
		zap.String("reasoning", selection.Reasoning),
		zap.String("provider", metadata.Provider),
	)

	if selection.SelectedIndex == -1 || selection.Confidence < 0.7 {
		e.logger.Warn("Low confidence in channel selection")
		return nil, nil
	}

	if selection.SelectedIndex < 0 || selection.SelectedIndex >= len(candidates) {
		e.logger.Error("Invalid selected index",
			zap.Int("index", selection.SelectedIndex),
			zap.Int("candidates", len(candidates)),
		)
		return nil, nil
	}

	return candidates[selection.SelectedIndex], nil
}

func parseAIResponse(rawResult any) (*domain.ParseResults, error) {
	if arr, ok := rawResult.([]any); ok {
		results := make([]*domain.ParseResult, 0, len(arr))
		for _, item := range arr {
			pr, err := parseResultObject(item)
			if err != nil {
				continue
			}
			results = append(results, pr)
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no valid results in array")
		}
		return &domain.ParseResults{Multiple: results}, nil
	}

	pr, err := parseResultObject(rawResult)
	if err != nil {
		return nil, err
	}
	return &domain.ParseResults{Single: pr}, nil
}

func parseResultObject(obj any) (*domain.ParseResult, error) {
	m, ok := obj.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object, got %T", obj)
	}

	pr := &domain.ParseResult{
		Params: make(map[string]any),
	}

	if cmd, ok := m["command"].(string); ok {
		pr.Command = domain.CommandType(strings.ToLower(cmd))
	} else {
		return nil, fmt.Errorf("missing command field")
	}

	if params, ok := m["params"].(map[string]any); ok {
		pr.Params = params
	}

	if conf, ok := m["confidence"].(float64); ok {
		pr.Confidence = conf
	}

	if reasoning, ok := m["reasoning"].(string); ok {
		pr.Reasoning = reasoning
	}

	return pr, nil
}

func unknownParseResult(reason string) *domain.ParseResults {
	return &domain.ParseResults{
		Single: &domain.ParseResult{
			Command:    domain.CommandUnknown,
			Params:     make(map[string]any),
			Confidence: 0,
			Reasoning:  reason,
		},
	}
}
