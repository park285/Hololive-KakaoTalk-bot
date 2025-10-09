package command

import (
	"context"
	"fmt"
	"strings"
	"unicode"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"go.uber.org/zap"
)

type AskCommand struct {
	deps *Dependencies
}

func NewAskCommand(deps *Dependencies) *AskCommand {
	return &AskCommand{deps: deps}
}

func (c *AskCommand) Name() string {
	return "ask"
}

func (c *AskCommand) Description() string {
	return "자연어 질의 처리"
}

func (c *AskCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	if c.deps == nil || c.deps.Gemini == nil || c.deps.MembersData == nil || c.deps.ExecuteCommand == nil {
		if c.deps != nil && c.deps.SendError != nil {
			return c.deps.SendError(cmdCtx.Room, "AI 서비스가 준비되지 않았습니다. 잠시 후 다시 시도해주세요.")
		}
		return fmt.Errorf("AI services not configured")
	}

	if c.deps.SendError == nil || c.deps.SendMessage == nil {
		return fmt.Errorf("message callbacks not configured")
	}

	if c.deps.Logger == nil {
		c.deps.Logger = zap.NewNop()
	}

	if cmdCtx == nil {
		return fmt.Errorf("command context is nil")
	}

	if ctx == nil {
		return fmt.Errorf("context is nil")
	}

	rawQuestion, _ := params["question"].(string)
	question := strings.TrimSpace(rawQuestion)
	if question == "" {
		return c.deps.SendError(cmdCtx.Room, "질문을 이해하지 못했습니다. 다시 입력해주세요.")
	}

	c.deps.Logger.Info("Processing natural language question", zap.String("question", question))

	parseResults, metadata, err := c.deps.Gemini.ParseNaturalLanguage(ctx, question, c.deps.MembersData)
	if err != nil {
		return c.deps.SendError(cmdCtx.Room, err.Error())
	}

	commands := parseResults.GetCommands()
	if len(commands) == 0 {
		if c.tryMemberInfo(ctx, cmdCtx, question) {
			return nil
		}
		return c.deps.SendMessage(cmdCtx.Room, "요청을 이해하지 못했습니다. ❓지원하지 않는 명령어입니다. !도움말 명령어를 참고해주세요.")
	}

	const minConfidence = 0.5

	validResults := make([]*domain.ParseResult, 0, len(commands))
	for _, result := range commands {
		if result == nil {
			continue
		}

		if result.Confidence < minConfidence {
			if c.deps.Logger != nil {
				c.deps.Logger.Warn("Skipping low-confidence command",
					zap.String("question", question),
					zap.String("command", result.Command.String()),
					zap.Float64("confidence", result.Confidence),
				)
			}
			continue
		}

		switch result.Command {
		case domain.CommandUnknown, domain.CommandAsk:
			if c.deps.Logger != nil {
				c.deps.Logger.Debug("Skipping non-actionable command",
					zap.String("command", result.Command.String()),
					zap.Float64("confidence", result.Confidence),
				)
			}
			continue
		}

		validResults = append(validResults, result)
	}

	if len(validResults) == 0 {
		if c.tryMemberInfo(ctx, cmdCtx, question) {
			return nil
		}
		return c.deps.SendMessage(cmdCtx.Room, "요청을 이해하지 못했습니다. ❓지원하지 않는 명령어입니다. !도움말 명령어를 참고해주세요.")
	}

	executed := 0
	for _, result := range validResults {
		if c.deps.Logger != nil {
			c.deps.Logger.Debug("Delegating parsed command",
				zap.String("question", question),
				zap.String("command", result.Command.String()),
				zap.Float64("confidence", result.Confidence),
				zap.Any("params", result.Params),
			)
		}

		forwardParams := make(map[string]any, len(result.Params))
		for k, v := range result.Params {
			forwardParams[k] = v
		}

		if err := c.deps.ExecuteCommand(ctx, cmdCtx, result.Command, forwardParams); err != nil {
			return err
		}

		executed++
	}

	if executed == 0 {
		if c.tryMemberInfo(ctx, cmdCtx, question) {
			return nil
		}
		return c.deps.SendMessage(cmdCtx.Room, "요청을 이해하지 못했습니다. ❓지원하지 않는 명령어입니다. !도움말 명령어를 참고해주세요.")
	}

	if metadata != nil && c.deps.Logger != nil {
		c.deps.Logger.Info("Natural language query processed",
			zap.String("provider", metadata.Provider),
			zap.String("model", metadata.Model),
			zap.Bool("used_fallback", metadata.UsedFallback),
			zap.Int("commands_executed", executed),
		)
	}

	return nil
}

func (c *AskCommand) tryMemberInfo(ctx context.Context, cmdCtx *domain.CommandContext, question string) bool {
	if c.deps == nil || c.deps.Matcher == nil || c.deps.ExecuteCommand == nil {
		return false
	}

	channel, err := c.deps.Matcher.FindBestMatch(ctx, question)
	var member *domain.Member
	if err == nil && channel != nil {
		member = c.deps.MembersData.FindMemberByChannelID(channel.ID)
	}

	if member == nil {
		member = c.findMember(question)
	}

	if member != nil {
		channelID := member.ChannelID
		if channel == nil && channelID != "" {
			if fetched, fetchErr := c.deps.Matcher.FindBestMatch(ctx, member.Name); fetchErr == nil && fetched != nil {
				channelID = fetched.ID
			}
		}

		params := map[string]any{
			"query":  question,
			"member": member.Name,
		}

		if channelID != "" {
			params["channel_id"] = channelID
		}

		if err := c.deps.ExecuteCommand(ctx, cmdCtx, domain.CommandMemberInfo, params); err != nil {
			if c.deps.Logger != nil {
				c.deps.Logger.Warn("Member info fallback failed",
					zap.String("question", question),
					zap.String("member", member.Name),
					zap.Error(err),
				)
			}
			return c.handleFallbackFail(ctx, cmdCtx, question)
		}

		if c.deps.Logger != nil {
			c.deps.Logger.Info("Member info fallback triggered",
				zap.String("question", question),
				zap.String("member", member.Name),
			)
		}

		return true
	}

	if c.handleFallbackFail(ctx, cmdCtx, question) {
		return true
	}
	return false
}

func hasExplicitMemberInfoKeyword(question string) bool {
	normalized := normalizeForMemberInfoIntent(question)

	for _, keyword := range memberInfoKeywordTokens {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}

	return false
}

func normalizeForMemberInfoIntent(input string) string {
	lower := util.Normalize(input)
	var builder strings.Builder
	builder.Grow(len(lower))

	for _, r := range lower {
		if unicode.IsSpace(r) {
			continue
		}

		switch r {
		case '.', ',', '!', '?', '"', '\'', '“', '”', '’', '‘', '、', '。', '·', '…', '-', '~', '～', '―', '–', '—', ':', ';':
			continue
		}

		builder.WriteRune(r)
	}

	return builder.String()
}

var memberInfoKeywordTokens = []string{
	"누구",
	"소개",
	"소개좀",
	"소개해",
	"정보",
	"정보좀",
	"프로필",
	"알려",
	"who",
	"whois",
	"whatis",
	"tellmeabout",
	"profile",
	"describe",
	"어떤사람",
	"정체",
}

func (c *AskCommand) findMember(question string) *domain.Member {
	if c.deps == nil || c.deps.MembersData == nil {
		return nil
	}

	lower := strings.ToLower(question)

	for _, member := range c.deps.MembersData.GetAllMembers() {
		if member == nil {
			continue
		}

		if name := strings.ToLower(member.Name); name != "" && strings.Contains(lower, name) {
			return member
		}

		if ja := strings.ToLower(member.NameJa); ja != "" && strings.Contains(lower, ja) {
			return member
		}

		for _, alias := range member.GetAllAliases() {
			if alias == "" {
				continue
			}
			if strings.Contains(lower, strings.ToLower(alias)) {
				return member
			}
		}
	}

	return nil
}

func (c *AskCommand) handleFallbackFail(ctx context.Context, cmdCtx *domain.CommandContext, question string) bool {
	if c.deps == nil || c.deps.SendMessage == nil {
		return false
	}

	sanitizedQuestion := strings.TrimSpace(question)
	var clarification string

	if c.deps.Gemini != nil && c.deps.MembersData != nil {
		response, metadata, err := c.deps.Gemini.GenerateSmartClarification(ctx, sanitizedQuestion, c.deps.MembersData)
		if err == nil && response != nil {
			if metadata != nil && c.deps.Logger != nil {
				c.deps.Logger.Info("Smart clarification generated",
					zap.String("question", sanitizedQuestion),
					zap.String("provider", metadata.Provider),
					zap.String("model", metadata.Model),
					zap.Bool("used_fallback", metadata.UsedFallback),
					zap.Bool("is_hololive_related", response.IsHololiveRelated),
				)
			}

			if !response.IsHololiveRelated {
				return false
			}

			if strings.TrimSpace(response.Message) != "" {
				clarification = strings.TrimSpace(response.Message)
			}
		} else if err != nil && c.deps.Logger != nil {
			c.deps.Logger.Warn("Failed to generate smart clarification",
				zap.String("question", sanitizedQuestion),
				zap.Error(err),
			)
		}
	}

	if clarification == "" {
		escaped := strings.ReplaceAll(sanitizedQuestion, `"`, "'")
		clarification = fmt.Sprintf("누구를 말씀하신 건지 잘 모르겠어요. \"%s\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요.", escaped)
	}

	if err := c.deps.SendMessage(cmdCtx.Room, clarification); err != nil {
		if c.deps.Logger != nil {
			c.deps.Logger.Error("Failed to send clarification message",
				zap.String("question", sanitizedQuestion),
				zap.Error(err),
			)
		}
		return false
	}

	return true
}
