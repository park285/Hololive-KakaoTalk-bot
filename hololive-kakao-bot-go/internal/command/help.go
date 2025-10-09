package command

import (
	"context"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

// HelpCommand handles help requests
type HelpCommand struct {
	deps *Dependencies
}

// NewHelpCommand creates a new HelpCommand
func NewHelpCommand(deps *Dependencies) *HelpCommand {
	return &HelpCommand{deps: deps}
}

// Name returns the command name
func (c *HelpCommand) Name() string {
	return "help"
}

// Description returns the command description
func (c *HelpCommand) Description() string {
	return "도움말을 표시합니다"
}

// Execute executes the help command
func (c *HelpCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	message := c.deps.Formatter.FormatHelp()
	return c.deps.SendMessage(cmdCtx.Room, message)
}
