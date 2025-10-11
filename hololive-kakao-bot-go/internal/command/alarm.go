package command

import (
	"context"
	"fmt"

	"github.com/kapu/hololive-kakao-bot-go/internal/adapter"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
)

type AlarmCommand struct {
	deps *Dependencies
}

func NewAlarmCommand(deps *Dependencies) *AlarmCommand {
	return &AlarmCommand{deps: deps}
}

func (c *AlarmCommand) Name() string {
	return "alarm"
}

func (c *AlarmCommand) Description() string {
	return "방송 알람 관리"
}

func (c *AlarmCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	if err := c.ensureDeps(); err != nil {
		return err
	}

	if c.deps.Alarm == nil {
		return c.deps.SendError(cmdCtx.Room, "알람 서비스가 초기화되지 않았습니다.")
	}

	action, hasAction := params["action"].(string)
	if !hasAction {
		action = "list"
	}

	switch action {
	case "set", "add":
		return c.handleAdd(ctx, cmdCtx, params)
	case "remove", "delete":
		return c.handleRemove(ctx, cmdCtx, params)
	case "list":
		c.deps.Logger.Info("Alarm list requested")
		return c.handleList(ctx, cmdCtx)
	case "clear":
		return c.handleClear(ctx, cmdCtx)
	default:
		return c.deps.SendMessage(cmdCtx.Room, c.deps.Formatter.FormatHelp())
	}
}

func (c *AlarmCommand) ensureDeps() error {
	if c == nil || c.deps == nil {
		return fmt.Errorf("alarm command dependencies not configured")
	}

	if c.deps.SendMessage == nil || c.deps.SendError == nil {
		return fmt.Errorf("message callbacks not configured")
	}

	if c.deps.Matcher == nil || c.deps.Formatter == nil {
		return fmt.Errorf("alarm command services not configured")
	}

	if c.deps.Logger == nil {
		c.deps.Logger = zap.NewNop()
	}

	return nil
}

func (c *AlarmCommand) handleAdd(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	memberName, hasMember := params["member"].(string)
	if !hasMember || memberName == "" {
		return c.deps.SendError(cmdCtx.Room, "멤버 이름을 입력해주세요.\n예) !알람 추가 페코라")
	}

	c.deps.Logger.Info("Alarm add requested", zap.String("member", memberName))

	channel, err := c.deps.Matcher.FindBestMatch(ctx, memberName)
	if err != nil {
		return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("멤버 검색 중 오류: %v", err))
	}

	if channel == nil {
		return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("'%s' 멤버를 찾을 수 없습니다.", memberName))
	}

	added, err := c.deps.Alarm.AddAlarm(ctx, cmdCtx.Room, cmdCtx.Sender, channel.ID, channel.Name)
	if err != nil {
		c.deps.Logger.Error("Failed to add alarm",
			zap.String("channel", channel.Name),
			zap.Error(err),
		)
		return c.deps.SendError(cmdCtx.Room, "알람 설정 중 오류가 발생했습니다.")
	}

	nextStreamInfo, _ := c.deps.Alarm.GetNextStreamInfo(ctx, channel.ID)

	message := c.deps.Formatter.FormatAlarmAdded(channel.Name, added, nextStreamInfo)
	return c.deps.SendMessage(cmdCtx.Room, message)
}

func (c *AlarmCommand) handleRemove(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	memberName, hasMember := params["member"].(string)
	if !hasMember || memberName == "" {
		return c.deps.SendError(cmdCtx.Room, "멤버 이름을 입력해주세요.\n예) !알람 제거 페코라")
	}

	c.deps.Logger.Info("Alarm remove requested", zap.String("member", memberName))

	channel, err := c.deps.Matcher.FindBestMatch(ctx, memberName)
	if err != nil {
		return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("멤버 검색 중 오류: %v", err))
	}

	if channel == nil {
		return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("'%s' 멤버를 찾을 수 없습니다.", memberName))
	}

	removed, err := c.deps.Alarm.RemoveAlarm(ctx, cmdCtx.Room, cmdCtx.Sender, channel.ID)
	if err != nil {
		c.deps.Logger.Error("Failed to remove alarm",
			zap.String("channel", channel.Name),
			zap.Error(err),
		)
		return c.deps.SendError(cmdCtx.Room, "알람 제거 중 오류가 발생했습니다.")
	}

	message := c.deps.Formatter.FormatAlarmRemoved(channel.Name, removed)
	return c.deps.SendMessage(cmdCtx.Room, message)
}

func (c *AlarmCommand) handleList(ctx context.Context, cmdCtx *domain.CommandContext) error {
	channelIDs, err := c.deps.Alarm.GetUserAlarms(ctx, cmdCtx.Room, cmdCtx.Sender)
	if err != nil {
		return c.deps.SendError(cmdCtx.Room, "알람 목록 조회 실패")
	}

	alarmInfos := make([]adapter.AlarmListEntry, 0, len(channelIDs))
	for _, channelID := range channelIDs {
		memberName, err := c.deps.Alarm.GetMemberName(ctx, channelID)
		if err != nil || memberName == "" {
			memberName = channelID
		}
		nextStreamInfo, _ := c.deps.Alarm.GetNextStreamInfo(ctx, channelID)
		alarmInfos = append(alarmInfos, adapter.AlarmListEntry{
			MemberName: memberName,
			NextStream: nextStreamInfo,
		})
	}

	message := c.deps.Formatter.FormatAlarmList(alarmInfos)
	return c.deps.SendMessage(cmdCtx.Room, message)
}

func (c *AlarmCommand) handleClear(ctx context.Context, cmdCtx *domain.CommandContext) error {
	count, err := c.deps.Alarm.ClearUserAlarms(ctx, cmdCtx.Room, cmdCtx.Sender)
	if err != nil {
		c.deps.Logger.Error("Failed to clear alarms", zap.Error(err))
		return c.deps.SendError(cmdCtx.Room, "알람 초기화 중 오류가 발생했습니다.")
	}

	message := c.deps.Formatter.FormatAlarmCleared(count)
	return c.deps.SendMessage(cmdCtx.Room, message)
}
