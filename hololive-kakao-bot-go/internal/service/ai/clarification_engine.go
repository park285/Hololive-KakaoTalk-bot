package ai

import (
	"context"
	"fmt"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/prompt"
	"go.uber.org/zap"
)

type ClarificationEngine struct {
	invoker       ModelInvoker
	promptBuilder *prompt.PromptBuilder
	memberCache   *MemberListCacheManager
	logger        *zap.Logger

	basicPreset    ModelPreset
	smartPreset    ModelPreset
	basicMaxTokens int
	smartMaxTokens int
}

func NewClarificationEngine(
	invoker ModelInvoker,
	builder *prompt.PromptBuilder,
	memberCache *MemberListCacheManager,
	logger *zap.Logger,
) *ClarificationEngine {
	return &ClarificationEngine{
		invoker:        invoker,
		promptBuilder:  builder,
		memberCache:    memberCache,
		logger:         logger,
		basicPreset:    PresetBalanced,
		smartPreset:    PresetBalanced,
		basicMaxTokens: 256,
		smartMaxTokens: 512,
	}
}

func (e *ClarificationEngine) GenerateBasic(ctx context.Context, query string) (string, *GenerateMetadata, error) {
	sanitized := sanitizeInput(query)
	if sanitized == "" {
		sanitized = strings.TrimSpace(query)
	}
	if sanitized == "" {
		return "", nil, fmt.Errorf("empty query for clarification")
	}

	data := prompt.ClarificationBasicData{
		UserQuery: sanitized,
	}
	promptText, err := e.promptBuilder.Render(prompt.TemplateClarificationBasic, data)
	if err != nil {
		e.logger.Error("Failed to render clarification (basic) template, using fallback", zap.Error(err))
		promptText = prompt.FallbackClarificationBasic(data)
	}

	var clarification domain.ClarificationResponse
	opts := &GenerateOptions{
		Overrides: &ModelConfig{MaxOutputTokens: e.basicMaxTokens},
	}

	metadata, err := e.invoker.GenerateJSON(ctx, promptText, e.basicPreset, &clarification, opts)
	if err != nil {
		e.logger.Error("Failed to generate clarification message", zap.Error(err))
		return "", nil, err
	}

	candidate := strings.TrimSpace(clarification.Candidate)
	message := strings.TrimSpace(clarification.Message)

	if candidate == "" {
		candidate = extractQuotedCandidate(message)
	}
	if candidate == "" {
		candidate = sanitized
	}

	finalMessage := buildClarificationSentence(candidate)

	if metadata != nil {
		e.logger.Info("Clarification message generated",
			zap.String("provider", metadata.Provider),
			zap.String("model", metadata.Model),
			zap.Bool("used_fallback", metadata.UsedFallback),
			zap.String("candidate", candidate),
		)
	}

	return finalMessage, metadata, nil
}

func (e *ClarificationEngine) GenerateSmart(ctx context.Context, query string, members domain.MemberDataProvider) (*domain.Clarification, *GenerateMetadata, error) {
	sanitized := sanitizeInput(query)
	if sanitized == "" {
		sanitized = strings.TrimSpace(query)
	}
	if sanitized == "" {
		return nil, nil, fmt.Errorf("empty query for smart clarification")
	}

	cachedName := ""
	if e.memberCache != nil {
		cachedName = e.memberCache.GetCachedName()
	}

	var promptText string
	var opts *GenerateOptions

	if cachedName != "" {
		data := prompt.ClarificationWithoutMembersData{
			UserQuery: sanitized,
		}
		rendered, err := e.promptBuilder.Render(prompt.TemplateClarificationWithoutMembers, data)
		if err != nil {
			e.logger.Error("Failed to render smart clarification template (cached), using fallback", zap.Error(err))
			rendered = prompt.FallbackClarificationWithoutMembers(data)
		}
		promptText = rendered
		opts = &GenerateOptions{
			Overrides: &ModelConfig{
				MaxOutputTokens: e.smartMaxTokens,
			},
			CachedContent: cachedName,
		}
	} else {
		memberNames := collectMemberNames(members)
		data := prompt.ClarificationWithMembersData{
			UserQuery:  sanitized,
			MemberList: strings.Join(memberNames, ", "),
		}
		rendered, err := e.promptBuilder.Render(prompt.TemplateClarificationWithMembers, data)
		if err != nil {
			e.logger.Error("Failed to render smart clarification template (embedded members), using fallback", zap.Error(err))
			rendered = prompt.FallbackClarificationWithMembers(data)
		}
		promptText = rendered
		opts = &GenerateOptions{
			Overrides: &ModelConfig{
				MaxOutputTokens: e.smartMaxTokens,
			},
		}
		e.logger.Warn("Member list cache not available, embedding in prompt")
	}

	var response domain.Clarification
	metadata, err := e.invoker.GenerateJSON(ctx, promptText, e.smartPreset, &response, opts)
	if err != nil {
		e.logger.Error("Failed to generate smart clarification", zap.Error(err))
		return nil, nil, err
	}

	if response.IsHololiveRelated && strings.TrimSpace(response.Message) == "" {
		candidate := strings.TrimSpace(response.Candidate)
		if candidate == "" {
			candidate = sanitized
		}
		response.Message = buildClarificationSentence(candidate)
	}

	if metadata != nil {
		e.logger.Info("Smart clarification generated",
			zap.String("provider", metadata.Provider),
			zap.String("model", metadata.Model),
			zap.Bool("used_fallback", metadata.UsedFallback),
			zap.Bool("is_hololive_related", response.IsHololiveRelated),
			zap.String("candidate", response.Candidate),
			zap.Bool("used_cache", cachedName != ""),
		)
	}

	return &response, metadata, nil
}

func collectMemberNames(members domain.MemberDataProvider) []string {
	if members == nil {
		return nil
	}
	all := members.GetAllMembers()
	names := make([]string, 0, len(all))
	for _, member := range all {
		if member != nil && member.Name != "" {
			names = append(names, member.Name)
		}
	}
	return names
}
