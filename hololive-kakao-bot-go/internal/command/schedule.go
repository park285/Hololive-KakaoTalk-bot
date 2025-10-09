package command

import (
	"context"
	"fmt"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

type ScheduleCommand struct {
	deps *Dependencies
}

func NewScheduleCommand(deps *Dependencies) *ScheduleCommand {
	return &ScheduleCommand{deps: deps}
}

func (c *ScheduleCommand) Name() string {
	return "schedule"
}

func (c *ScheduleCommand) Description() string {
	return "특정 멤버 일정 조회"
}

func (c *ScheduleCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	memberName, hasMember := params["member"].(string)
	if !hasMember || memberName == "" {
		return c.deps.SendError(cmdCtx.Room, "멤버 이름을 지정해주세요.\n예) !일정 페코라")
	}
	days := 7
	if d, ok := params["days"]; ok {
		switch v := d.(type) {
		case float64:
			days = int(v)
		case int:
			days = v
		}
	}

	if days < 1 {
		days = 7
	}
	if days > 30 {
		days = 30
	}

	channel, err := c.deps.Matcher.FindBestMatch(ctx, memberName)
	if err != nil {
		return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("멤버 검색 중 오류: %v", err))
	}

	if channel == nil {
		return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("'%s' 멤버를 찾을 수 없습니다.", memberName))
	}

	hours := days * 24
	streams, err := c.deps.Holodex.GetChannelSchedule(ctx, channel.ID, hours, true)
	if err != nil {
		return c.deps.SendError(cmdCtx.Room, "일정 조회 실패")
	}

	message := c.deps.Formatter.FormatChannelSchedule(channel, streams, days)
	return c.deps.SendMessage(cmdCtx.Room, message)
}
