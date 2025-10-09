package command

import (
	"context"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

type HelpCommand struct {
	deps *Dependencies
}

func NewHelpCommand(deps *Dependencies) *HelpCommand {
	return &HelpCommand{deps: deps}
}

func (c *HelpCommand) Name() string {
	return "help"
}

func (c *HelpCommand) Description() string {
	return "도움말을 표시합니다"
}

func (c *HelpCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	message := c.deps.Formatter.FormatHelp()
	return c.deps.SendMessage(cmdCtx.Room, message)
}
