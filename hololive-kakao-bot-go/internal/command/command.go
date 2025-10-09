package command

import (
	"context"

	"github.com/kapu/hololive-kakao-bot-go/internal/adapter"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/service"
	"go.uber.org/zap"
)

// Command defines the command interface
type Command interface {
	Name() string
	Description() string
	Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error
}

// NaturalLanguageParser represents a service capable of parsing natural language queries into structured commands.
type NaturalLanguageParser interface {
	ParseNaturalLanguage(ctx context.Context, query string, membersData *domain.MembersData) (*domain.ParseResults, *service.GenerateMetadata, error)
	GenerateClarificationMessage(ctx context.Context, query string) (string, *service.GenerateMetadata, error)
	ClassifyMemberInfoIntent(ctx context.Context, query string) (*domain.MemberIntentClassification, *service.GenerateMetadata, error)
	GenerateSmartClarification(ctx context.Context, query string, membersData *domain.MembersData) (*domain.SmartClarificationResponse, *service.GenerateMetadata, error)
}

// Dependencies holds all command dependencies
type Dependencies struct {
	Holodex          *service.HolodexService
	Cache            *service.CacheService
	Gemini           NaturalLanguageParser
	Alarm            *service.AlarmService
	Matcher          *service.MemberMatcher
	OfficialProfiles *service.OfficialProfileService
	MembersData      *domain.MembersData
	Formatter        *adapter.ResponseFormatter
	SendMessage      func(room, message string) error
	SendError        func(room, message string) error
	ExecuteCommand   func(ctx context.Context, cmdCtx *domain.CommandContext, cmdType domain.CommandType, params map[string]any) error
	Logger           *zap.Logger
}
