package ai

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/prompt"
	"go.uber.org/zap"
)

type GeminiService struct {
	modelManager      *ModelManager
	logger            *zap.Logger
	promptBuilder     *prompt.PromptBuilder
	directoryBuilder  *MemberDirectoryBuilder
	memberCache       *MemberListCacheManager
	parseCache        *ParseCache
	parseEngine       *ParseEngine
	clarification     *ClarificationEngine
	memberCacheTicker bool
}

func NewGeminiService(modelManager *ModelManager, logger *zap.Logger) *GeminiService {
	builder := prompt.NewPromptBuilder()
	directoryBuilder := NewMemberDirectoryBuilder()
	parseCache := NewParseCache(5 * time.Minute)
	memberCache := NewMemberListCacheManager(modelManager, logger)

	parseEngine := NewParseEngine(modelManager, builder, directoryBuilder, parseCache, logger)
	clarification := NewClarificationEngine(modelManager, builder, memberCache, logger)

	return &GeminiService{
		modelManager:     modelManager,
		logger:           logger,
		promptBuilder:    builder,
		directoryBuilder: directoryBuilder,
		memberCache:      memberCache,
		parseCache:       parseCache,
		parseEngine:      parseEngine,
		clarification:    clarification,
	}
}

func (gs *GeminiService) InitializeMemberListCache(ctx context.Context, membersData domain.MemberDataProvider) error {
	if err := gs.memberCache.EnsureInitialized(ctx, membersData); err != nil {
		return err
	}
	if !gs.memberCacheTicker {
		gs.memberCache.StartAutoRefresh(ctx, membersData)
		gs.memberCacheTicker = true
	}
	return nil
}

func (gs *GeminiService) ParseNaturalLanguage(ctx context.Context, query string, membersData domain.MemberDataProvider) (*domain.ParseResults, *GenerateMetadata, error) {
	return gs.parseEngine.Parse(ctx, query, membersData)
}

func (gs *GeminiService) SelectBestChannel(ctx context.Context, query string, candidates []*domain.Channel) (*domain.Channel, error) {
	return gs.parseEngine.SelectBestChannel(ctx, query, candidates)
}

func (gs *GeminiService) GenerateClarificationMessage(ctx context.Context, query string) (string, *GenerateMetadata, error) {
	return gs.clarification.GenerateBasic(ctx, query)
}

func (gs *GeminiService) GenerateSmartClarification(ctx context.Context, query string, membersData domain.MemberDataProvider) (*domain.Clarification, *GenerateMetadata, error) {
	return gs.clarification.GenerateSmart(ctx, query, membersData)
}

func (gs *GeminiService) GetMemberListCacheName() string {
	return gs.memberCache.GetCachedName()
}

func (gs *GeminiService) InvalidateMemberCache() {
	gs.directoryBuilder.Invalidate()
	gs.memberCache.Invalidate()
	gs.memberCacheTicker = false
	gs.logger.Info("Member list cache invalidated")
}

func (gs *GeminiService) ClassifyMemberInfoIntent(ctx context.Context, query string) (*domain.MemberIntent, *GenerateMetadata, error) {
	sanitized := sanitizeInput(query)
	if sanitized == "" {
		return &domain.MemberIntent{
			Intent:     domain.MemberIntentOther,
			Confidence: 1.0,
			Reasoning:  "Empty query",
		}, &GenerateMetadata{Provider: "None", Model: "n/a"}, nil
	}

	memberKeywords := []string{"누구", "알려", "소개", "프로필", "정보", "멤버"}
	lowerQuery := strings.ToLower(sanitized)

	for _, keyword := range memberKeywords {
		if strings.Contains(lowerQuery, keyword) {
			return &domain.MemberIntent{
				Intent:     domain.MemberIntentMemberInfo,
				Confidence: 0.8,
				Reasoning:  fmt.Sprintf("Contains keyword: %s", keyword),
			}, &GenerateMetadata{Provider: "Rule", Model: "keyword-match"}, nil
		}
	}

	return &domain.MemberIntent{
		Intent:     domain.MemberIntentOther,
		Confidence: 0.6,
		Reasoning:  "No member info keywords detected",
	}, &GenerateMetadata{Provider: "Rule", Model: "keyword-match"}, nil
}

func (gs *GeminiService) Close() {
	gs.memberCache.Stop()
	gs.memberCacheTicker = false
}
