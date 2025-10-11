package command

import (
	"context"

	"github.com/kapu/hololive-kakao-bot-go/internal/adapter"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/ai"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/cache"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/holodex"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/matcher"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/member"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/notification"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/youtube"
	"go.uber.org/zap"
)

type Command interface {
	Name() string
	Description() string
	Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error
}

// CommandEvent represents an instruction to execute another command. It is used
// by dispatcher-enabled commands such as natural language handlers to delegate
// follow-up actions in a structured way.
type CommandEvent struct {
	Type   domain.CommandType
	Params map[string]any
}

// Dispatcher coordinates execution of one or more command events.
type Dispatcher interface {
	Publish(ctx context.Context, cmdCtx *domain.CommandContext, events ...CommandEvent) (int, error)
}

type NaturalLanguageParser interface {
	ParseNaturalLanguage(ctx context.Context, query string, membersData domain.MemberDataProvider) (*domain.ParseResults, *ai.GenerateMetadata, error)
	GenerateClarificationMessage(ctx context.Context, query string) (string, *ai.GenerateMetadata, error)
	ClassifyMemberInfoIntent(ctx context.Context, query string) (*domain.MemberIntent, *ai.GenerateMetadata, error)
	GenerateSmartClarification(ctx context.Context, query string, membersData domain.MemberDataProvider) (*domain.Clarification, *ai.GenerateMetadata, error)
}

type Dependencies struct {
	Holodex          *holodex.HolodexService
	Cache            *cache.CacheService
	Gemini           NaturalLanguageParser
	Alarm            *notification.AlarmService
	Matcher          *matcher.MemberMatcher
	OfficialProfiles *member.ProfileService
	StatsRepo        *youtube.YouTubeStatsRepository
	MembersData      domain.MemberDataProvider
	Formatter        *adapter.ResponseFormatter
	SendMessage      func(room, message string) error
	SendError        func(room, message string) error
	Dispatcher       Dispatcher
	Logger           *zap.Logger
}
