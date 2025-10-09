package command

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
)

type StatsCommand struct {
	deps *Dependencies
}

func NewStatsCommand(deps *Dependencies) *StatsCommand {
	return &StatsCommand{deps: deps}
}

func (c *StatsCommand) Name() string {
	return "stats"
}

func (c *StatsCommand) Description() string {
	return "구독자 순위 및 통계 조회"
}

func (c *StatsCommand) Execute(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	if c.deps == nil || c.deps.StatsRepo == nil {
		return c.deps.SendError(cmdCtx.Room, "통계 기능이 활성화되지 않았습니다.")
	}

	if c.deps.SendError == nil || c.deps.SendMessage == nil {
		return fmt.Errorf("message callbacks not configured")
	}

	if c.deps.Logger == nil {
		c.deps.Logger = zap.NewNop()
	}

	action, _ := params["action"].(string)
	if action == "" {
		action = "gainers"
	}

	switch strings.ToLower(action) {
	case "gainers", "구독자순위":
		return c.showTopGainers(ctx, cmdCtx, params)
	default:
		return c.deps.SendError(cmdCtx.Room, "알 수 없는 통계 유형입니다. !도움말을 참고해주세요.")
	}
}

func (c *StatsCommand) showTopGainers(ctx context.Context, cmdCtx *domain.CommandContext, params map[string]any) error {
	periodStr, _ := params["period"].(string)
	var since time.Time

	switch strings.ToLower(periodStr) {
	case "오늘", "today":
		since = time.Now().Add(-24 * time.Hour)
	case "주간", "week":
		since = time.Now().Add(-7 * 24 * time.Hour)
	case "월간", "month":
		since = time.Now().Add(-30 * 24 * time.Hour)
	default:
		since = time.Now().Add(-7 * 24 * time.Hour) // Default: 7 days
	}

	gainers, err := c.deps.StatsRepo.GetTopGainers(ctx, since, 10)
	if err != nil {
		c.deps.Logger.Error("Failed to get top gainers", zap.Error(err))
		return c.deps.SendError(cmdCtx.Room, "구독자 순위 조회 중 오류가 발생했습니다.")
	}

	if len(gainers) == 0 {
		return c.deps.SendMessage(cmdCtx.Room, "해당 기간의 통계 데이터가 없습니다.")
	}

	var builder strings.Builder
	builder.WriteString("📊 구독자 증가 순위 (지난 7일)\n\n")

	for _, entry := range gainers {
		builder.WriteString(fmt.Sprintf("%d위. %s\n", entry.Rank, entry.MemberName))
		builder.WriteString(fmt.Sprintf("    +%s명\n\n", formatSubscriberGain(entry.Value)))
	}

	return c.deps.SendMessage(cmdCtx.Room, builder.String())
}

func formatSubscriberGain(n int64) string {
	if n >= 10000 {
		man := n / 10000
		remainder := n % 10000
		if remainder == 0 {
			return fmt.Sprintf("%d만", man)
		}
		return fmt.Sprintf("%d만 %d", man, remainder)
	}
	return fmt.Sprintf("%d", n)
}
