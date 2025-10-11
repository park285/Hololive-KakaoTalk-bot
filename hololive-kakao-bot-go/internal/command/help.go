package command

import (
	"context"
	"fmt"

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
	if err := c.ensureDeps(); err != nil {
		return err
	}
	message := c.deps.Formatter.FormatHelp()
	return c.deps.SendMessage(cmdCtx.Room, message)
}

func (c *HelpCommand) ensureDeps() error {
	if c == nil || c.deps == nil {
		return fmt.Errorf("help command dependencies not configured")
	}

	if c.deps.SendMessage == nil {
		return fmt.Errorf("message callback not configured")
	}

	if c.deps.Formatter == nil {
		return fmt.Errorf("formatter not configured")
	}

	return nil
}
