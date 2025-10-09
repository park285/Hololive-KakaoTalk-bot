package command

import (
	"context"

	"github.com/kapu/hololive-kakao-bot-go/internal/adapter"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/service"
	"go.uber.org/zap"
)

type Command interface {
	Name() string
	Description() string
	Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error
}

type NaturalLanguageParser interface {
	ParseNaturalLanguage(ctx context.Context, query string, membersData domain.MemberDataProvider) (*domain.ParseResults, *service.GenerateMetadata, error)
	GenerateClarificationMessage(ctx context.Context, query string) (string, *service.GenerateMetadata, error)
	ClassifyMemberInfoIntent(ctx context.Context, query string) (*domain.MemberIntent, *service.GenerateMetadata, error)
	GenerateSmartClarification(ctx context.Context, query string, membersData domain.MemberDataProvider) (*domain.Clarification, *service.GenerateMetadata, error)
}

type Dependencies struct {
	Holodex          *service.HolodexService
	Cache            *service.CacheService
	Gemini           NaturalLanguageParser
	Alarm            *service.AlarmService
	Matcher          *service.MemberMatcher
	OfficialProfiles *service.ProfileService
	StatsRepo        *service.YouTubeStatsRepository
	MembersData      domain.MemberDataProvider
	Formatter        *adapter.ResponseFormatter
	SendMessage      func(room, message string) error
	SendError        func(room, message string) error
	ExecuteCommand   func(ctx context.Context, cmdCtx *domain.CommandContext, cmdType domain.CommandType, params map[string]any) error
	Logger           *zap.Logger
}
