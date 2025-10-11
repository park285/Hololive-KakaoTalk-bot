package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/ai"
	"go.uber.org/zap"
)

type AskCommand struct {
	deps *Dependencies
}

type askWorkflow struct {
	ctx      context.Context
	deps     *Dependencies
	cmdCtx   *domain.CommandContext
	provider domain.MemberDataProvider
	logger   *zap.Logger
	dispatch Dispatcher
}

const (
	askUnsupportedMessage = "요청을 이해하지 못했습니다. ❓지원하지 않는 명령어입니다. !도움말 명령어를 참고해주세요."
	minAskConfidence      = 0.5
)

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
	workflow, err := c.prepareWorkflow(ctx, cmdCtx)
	if err != nil {
		return err
	}

	question := c.extractQuestion(params)
	if question == "" {
		return workflow.deps.SendError(cmdCtx.Room, "질문을 이해하지 못했습니다. 다시 입력해주세요.")
	}

	workflow.logger.Info("Processing natural language question", zap.String("question", question))

	parseResults, metadata, err := workflow.deps.Gemini.ParseNaturalLanguage(ctx, question, workflow.provider)
	if err != nil {
		return workflow.deps.SendError(cmdCtx.Room, err.Error())
	}

	validResults := workflow.filterValidResults(question, parseResults.GetCommands())
	if len(validResults) == 0 {
		return workflow.handleNoCommand(question)
	}

	executed, err := workflow.dispatchCommands(question, validResults)
	if err != nil {
		return err
	}

	if executed == 0 {
		return workflow.handleNoCommand(question)
	}

	workflow.logMetadata(metadata, executed)
	return nil
}

func (c *AskCommand) prepareWorkflow(ctx context.Context, cmdCtx *domain.CommandContext) (*askWorkflow, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context is nil")
	}

	if cmdCtx == nil {
		return nil, fmt.Errorf("command context is nil")
	}

	if c == nil || c.deps == nil {
		return nil, fmt.Errorf("ask command dependencies not configured")
	}

	if c.deps.SendError == nil || c.deps.SendMessage == nil {
		return nil, fmt.Errorf("message callbacks not configured")
	}

	if c.deps.Gemini == nil || c.deps.MembersData == nil || c.deps.Dispatcher == nil {
		return nil, c.deps.SendError(cmdCtx.Room, "AI 서비스가 준비되지 않았습니다. 잠시 후 다시 시도해주세요.")
	}

	logger := c.deps.Logger
	if logger == nil {
		logger = zap.NewNop()
	}

	provider := c.deps.MembersData.WithContext(ctx)

	return &askWorkflow{
		ctx:      ctx,
		deps:     c.deps,
		cmdCtx:   cmdCtx,
		provider: provider,
		logger:   logger,
		dispatch: c.deps.Dispatcher,
	}, nil
}

func (c *AskCommand) extractQuestion(params map[string]any) string {
	rawQuestion, _ := params["question"].(string)
	return strings.TrimSpace(rawQuestion)
}

func (w *askWorkflow) filterValidResults(question string, commands []*domain.ParseResult) []*domain.ParseResult {
	validResults := make([]*domain.ParseResult, 0, len(commands))

	for _, result := range commands {
		if result == nil {
			continue
		}

		if result.Confidence < minAskConfidence {
			w.logger.Warn("Skipping low-confidence command",
				zap.String("question", question),
				zap.String("command", result.Command.String()),
				zap.Float64("confidence", result.Confidence),
			)
			continue
		}

		switch result.Command {
		case domain.CommandUnknown, domain.CommandAsk:
			w.logger.Debug("Skipping non-actionable command",
				zap.String("command", result.Command.String()),
				zap.Float64("confidence", result.Confidence),
			)
			continue
		}

		validResults = append(validResults, result)
	}

	return validResults
}

func (w *askWorkflow) dispatchCommands(question string, results []*domain.ParseResult) (int, error) {
	events := make([]CommandEvent, 0, len(results))
	for _, result := range results {
		w.logger.Debug("Dispatching parsed command",
			zap.String("question", question),
			zap.String("command", result.Command.String()),
			zap.Float64("confidence", result.Confidence),
			zap.Any("params", result.Params),
		)

		events = append(events, CommandEvent{
			Type:   result.Command,
			Params: cloneParams(result.Params),
		})
	}
	return w.dispatch.Publish(w.ctx, w.cmdCtx, events...)
}

func (w *askWorkflow) handleNoCommand(question string) error {
	if w.tryMemberInfo(question) {
		return nil
	}
	return w.deps.SendMessage(w.cmdCtx.Room, askUnsupportedMessage)
}

func (w *askWorkflow) logMetadata(metadata *ai.GenerateMetadata, executed int) {
	if metadata == nil || w.logger == nil {
		return
	}

	w.logger.Info("Natural language query processed",
		zap.String("provider", metadata.Provider),
		zap.String("model", metadata.Model),
		zap.Bool("used_fallback", metadata.UsedFallback),
		zap.Int("commands_executed", executed),
	)
}

func (w *askWorkflow) tryMemberInfo(question string) bool {
	if w.deps == nil || w.deps.Matcher == nil || w.dispatch == nil {
		return false
	}

	channel, err := w.deps.Matcher.FindBestMatch(w.ctx, question)
	var member *domain.Member
	if err == nil && channel != nil {
		member = w.provider.FindMemberByChannelID(channel.ID)
	}

	if member == nil {
		member = w.findMember(question)
	}

	if member != nil {
		channelID := member.ChannelID
		if channel == nil && channelID != "" {
			if fetched, fetchErr := w.deps.Matcher.FindBestMatch(w.ctx, member.Name); fetchErr == nil && fetched != nil {
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

		if err := w.invokeCommand(domain.CommandMemberInfo, params); err != nil {
			if w.logger != nil {
				w.logger.Warn("Member info fallback failed",
					zap.String("question", question),
					zap.String("member", member.Name),
					zap.Error(err),
				)
			}
			return w.handleFallbackFail(question)
		}

		if w.logger != nil {
			w.logger.Info("Member info fallback triggered",
				zap.String("question", question),
				zap.String("member", member.Name),
			)
		}

		return true
	}

	if w.handleFallbackFail(question) {
		return true
	}
	return false
}

func (w *askWorkflow) invokeCommand(cmdType domain.CommandType, params map[string]any) error {
	_, err := w.dispatch.Publish(w.ctx, w.cmdCtx, CommandEvent{Type: cmdType, Params: cloneParams(params)})
	return err
}

func (w *askWorkflow) findMember(question string) *domain.Member {
	if w.provider == nil {
		return nil
	}

	lower := strings.ToLower(question)

	for _, member := range w.provider.GetAllMembers() {
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

func (w *askWorkflow) handleFallbackFail(question string) bool {
	if w.deps == nil || w.deps.SendMessage == nil {
		return false
	}

	sanitizedQuestion := strings.TrimSpace(question)
	var clarification string

	if w.deps.Gemini != nil && w.deps.MembersData != nil {
		response, metadata, err := w.deps.Gemini.GenerateSmartClarification(w.ctx, sanitizedQuestion, w.provider)
		if err == nil && response != nil {
			if metadata != nil && w.logger != nil {
				w.logger.Info("Smart clarification generated",
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
		} else if err != nil && w.logger != nil {
			w.logger.Warn("Failed to generate smart clarification",
				zap.String("question", sanitizedQuestion),
				zap.Error(err),
			)
		}
	}

	if clarification == "" {
		escaped := strings.ReplaceAll(sanitizedQuestion, `"`, "'")
		clarification = fmt.Sprintf("누구를 말씀하신 건지 잘 모르겠어요. \"%s\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요.", escaped)
	}

	if err := w.deps.SendMessage(w.cmdCtx.Room, clarification); err != nil {
		if w.logger != nil {
			w.logger.Error("Failed to send clarification message",
				zap.String("question", sanitizedQuestion),
				zap.Error(err),
			)
		}
		return false
	}

	return true
}
