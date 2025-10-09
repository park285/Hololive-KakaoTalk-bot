package command

import (
	"context"
	"fmt"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
)

type MemberInfoCommand struct {
	deps *Dependencies
}

func NewMemberInfoCommand(deps *Dependencies) *MemberInfoCommand {
	return &MemberInfoCommand{deps: deps}
}

func (c *MemberInfoCommand) Name() string {
	return string(domain.CommandMemberInfo)
}

func (c *MemberInfoCommand) Description() string {
	return "홀로라이브 멤버 공식 프로필"
}

func (c *MemberInfoCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	if c.deps == nil ||
		c.deps.Matcher == nil ||
		c.deps.MembersData == nil ||
		c.deps.Formatter == nil ||
		c.deps.SendMessage == nil ||
		c.deps.SendError == nil ||
		c.deps.OfficialProfiles == nil {
		return fmt.Errorf("member info command dependencies not satisfied")
	}

	rawQuery := getStringParam(params, "query")
	englishCandidate := getStringParam(params, "member")
	channelID := getStringParam(params, "channel_id")

	member := c.resolveMember(ctx, channelID, englishCandidate, rawQuery)
	if member == nil {
		if rawQuery == "" && englishCandidate == "" {
			return c.deps.SendError(cmdCtx.Room, "멤버 이름을 입력해주세요.")
		}
		target := englishCandidate
		if target == "" {
			target = rawQuery
		}
		return c.deps.SendError(cmdCtx.Room, c.deps.Formatter.FormatMemberNotFound(target))
	}

	rawProfile, translated, err := c.deps.OfficialProfiles.GetWithTranslation(ctx, member.Name)
	if err != nil {
		c.log().Error("Failed to load member profile",
			zap.String("member", member.Name),
			zap.Error(err),
		)
		return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("'%s' 프로필을 불러오는 중 오류가 발생했습니다.", member.Name))
	}

	message := c.deps.Formatter.FormatTalentProfile(rawProfile, translated)
	if message == "" {
		return c.deps.SendError(cmdCtx.Room, fmt.Sprintf("'%s' 프로필을 구성하지 못했습니다.", member.Name))
	}

	if member.IsGraduated {
		message = "⚠️ 졸업한 멤버입니다.\n\n" + message
	}

	return c.deps.SendMessage(cmdCtx.Room, message)
}

func (c *MemberInfoCommand) resolveMember(ctx context.Context, channelID, englishName, query string) *domain.Member {
	if channelID != "" {
		if member := c.deps.MembersData.FindMemberByChannelID(channelID); member != nil {
			return member
		}
	}

	if englishName != "" {
		if member := c.deps.MembersData.FindMemberByName(englishName); member != nil {
			return member
		}
	}

	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return nil
	}

	channel, err := c.deps.Matcher.FindBestMatch(ctx, trimmed)
	if err != nil {
		c.log().Warn("Member match failed",
			zap.String("query", trimmed),
			zap.Error(err),
		)
		return nil
	}
	if channel == nil {
		return nil
	}

	return c.deps.MembersData.FindMemberByChannelID(channel.ID)
}

func (c *MemberInfoCommand) log() *zap.Logger {
	if c.deps != nil && c.deps.Logger != nil {
		return c.deps.Logger
	}
	return zap.NewNop()
}

func getStringParam(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	val, ok := params[key]
	if !ok {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}
