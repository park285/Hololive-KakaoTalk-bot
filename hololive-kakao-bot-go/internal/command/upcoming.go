package command

import (
	"context"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

type UpcomingCommand struct {
	deps *Dependencies
}

func NewUpcomingCommand(deps *Dependencies) *UpcomingCommand {
	return &UpcomingCommand{deps: deps}
}

func (c *UpcomingCommand) Name() string {
	return "upcoming"
}

func (c *UpcomingCommand) Description() string {
	return "예정된 방송 목록"
}

func (c *UpcomingCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	hours := 24 // Default

	if h, ok := params["hours"]; ok {
		switch v := h.(type) {
		case float64:
			hours = int(v)
		case int:
			hours = v
		}
	}

	if hours < 1 {
		hours = 24
	}
	if hours > 168 {
		hours = 168
	}

	streams, err := c.deps.Holodex.GetUpcomingStreams(ctx, hours)
	if err != nil {
		return c.deps.SendError(cmdCtx.Room, "예정 방송 조회 실패")
	}

	message := c.deps.Formatter.FormatUpcomingStreams(streams, hours)
	return c.deps.SendMessage(cmdCtx.Room, message)
}
