package command

import (
	"context"
	"fmt"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

type LiveCommand struct {
	deps *Dependencies
}

func NewLiveCommand(deps *Dependencies) *LiveCommand {
	return &LiveCommand{deps: deps}
}

func (c *LiveCommand) Name() string {
	return "live"
}

func (c *LiveCommand) Description() string {
	return "현재 방송 중인 스트림 목록"
}

func (c *LiveCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	if err := c.ensureDeps(); err != nil {
		return err
	}

	memberName, hasMember := params["member"].(string)

	if hasMember && memberName != "" {
		channel, err := c.deps.Matcher.FindBestMatch(ctx, memberName)
		if err != nil {
			return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("멤버 검색 중 오류: %v", err))
		}

		if channel == nil {
			return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("'%s' 멤버를 찾을 수 없습니다.", memberName))
		}

		streams, err := c.deps.Holodex.GetLiveStreams(ctx)
		if err != nil {
			return c.deps.SendError(cmdCtx.Room, "라이브 스트림 조회 실패")
		}

		memberStreams := make([]*domain.Stream, 0, len(streams))
		for _, stream := range streams {
			if stream.ChannelID == channel.ID {
				memberStreams = append(memberStreams, stream)
			}
		}

		if len(memberStreams) == 0 {
			return c.deps.SendMessage(cmdCtx.Room, fmt.Sprintf("%s은(는) 현재 방송 중이 아닙니다.", channel.Name))
		}

		message := c.deps.Formatter.FormatLiveStreams(memberStreams)
		return c.deps.SendMessage(cmdCtx.Room, message)
	}

	streams, err := c.deps.Holodex.GetLiveStreams(ctx)
	if err != nil {
		return c.deps.SendError(cmdCtx.Room, "라이브 스트림 조회 실패")
	}

	message := c.deps.Formatter.FormatLiveStreams(streams)
	return c.deps.SendMessage(cmdCtx.Room, message)
}

func (c *LiveCommand) ensureDeps() error {
	if c == nil || c.deps == nil {
		return fmt.Errorf("live command dependencies not configured")
	}

	if c.deps.SendMessage == nil || c.deps.SendError == nil {
		return fmt.Errorf("message callbacks not configured")
	}

	if c.deps.Matcher == nil || c.deps.Holodex == nil || c.deps.Formatter == nil {
		return fmt.Errorf("live command services not configured")
	}

	return nil
}
