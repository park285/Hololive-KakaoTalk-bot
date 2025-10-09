package adapter

import (
	"fmt"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
)

// AlarmListEntry represents a single alarm item for formatting.
type AlarmListEntry struct {
	MemberName string
	NextStream string
}

// ResponseFormatter formats bot responses
type ResponseFormatter struct {
	prefix string
}

// NewResponseFormatter creates a new ResponseFormatter
func NewResponseFormatter(prefix string) *ResponseFormatter {
	if strings.TrimSpace(prefix) == "" {
		prefix = "!"
	}
	return &ResponseFormatter{prefix: prefix}
}

// FormatLiveStreams formats live streams into a message
func (f *ResponseFormatter) FormatLiveStreams(streams []*domain.Stream) string {
	if len(streams) == 0 {
		return "🔴 현재 방송 중인 스트림이 없습니다."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔴 현재 라이브 중 (%d개)\n\n", len(streams)))

	for i, stream := range streams {
		if i > 0 {
			sb.WriteString("\n")
		}

		title := f.truncateTitle(stream.Title)

		sb.WriteString(fmt.Sprintf("📺 %s\n", stream.ChannelName))
		sb.WriteString(fmt.Sprintf("   %s\n", title))
		sb.WriteString(fmt.Sprintf("   %s", stream.GetYouTubeURL()))

		if i < len(streams)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// FormatUpcomingStreams formats upcoming streams into a message
func (f *ResponseFormatter) FormatUpcomingStreams(streams []*domain.Stream, hours int) string {
	if len(streams) == 0 {
		return fmt.Sprintf("📅 %d시간 이내 예정된 방송이 없습니다.", hours)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 예정된 방송 (%d시간 이내, %d개)\n\n", hours, len(streams)))

	for i, stream := range streams {
		if i > 0 {
			sb.WriteString("\n")
		}

		title := f.truncateTitle(stream.Title)
		timeInfo := f.formatStreamTimeInfo(stream)

		sb.WriteString(fmt.Sprintf("📺 %s\n", stream.ChannelName))
		sb.WriteString(fmt.Sprintf("   %s\n", title))
		sb.WriteString(fmt.Sprintf("   ⏰ %s\n", timeInfo))
		sb.WriteString(fmt.Sprintf("   %s", stream.GetYouTubeURL()))

		if i < len(streams)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// FormatChannelSchedule formats channel schedule into a message
func (f *ResponseFormatter) FormatChannelSchedule(channel *domain.Channel, streams []*domain.Stream, days int) string {
	if channel == nil {
		return "❌ 채널 정보를 찾을 수 없습니다."
	}

	channelName := channel.GetDisplayName()

	if len(streams) == 0 {
		return fmt.Sprintf("📅 %s\n%d일 이내 예정된 방송이 없습니다.", channelName, days)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 %s 일정 (%d일 이내, %d개)\n\n", channelName, days, len(streams)))

	for i, stream := range streams {
		if i > 0 {
			sb.WriteString("\n")
		}

		title := f.truncateTitle(stream.Title)

		var statusIcon string
		var timeInfo string

		if stream.IsLive() {
			statusIcon = "🔴 LIVE"
			timeInfo = "지금 방송 중"
		} else {
			statusIcon = "⏰"
			timeInfo = f.formatStreamTimeInfo(stream)
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", statusIcon, title))
		sb.WriteString(fmt.Sprintf("   %s\n", timeInfo))
		sb.WriteString(fmt.Sprintf("   %s", stream.GetYouTubeURL()))

		if i < len(streams)-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

// FormatAlarmAdded formats alarm added confirmation
func (f *ResponseFormatter) FormatAlarmAdded(memberName string, added bool, nextStreamInfo string) string {
	if !added {
		return fmt.Sprintf("ℹ️ %s 알람이 이미 설정되어 있습니다.", memberName)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ %s 알람이 설정되었습니다!\n\n", memberName))
	if nextStreamInfo != "" {
		sb.WriteString(nextStreamInfo)
		if !strings.HasSuffix(nextStreamInfo, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("방송 시작 5분 전에 알림을 받습니다.\n")
	sb.WriteString(fmt.Sprintf("%s알람 목록 으로 확인 가능합니다.", f.prefix))
	return sb.String()
}

// FormatAlarmRemoved formats alarm removed confirmation
func (f *ResponseFormatter) FormatAlarmRemoved(memberName string, removed bool) string {
	if removed {
		return fmt.Sprintf("✅ %s 알람이 해제되었습니다.", memberName)
	}
	return fmt.Sprintf("❌ %s 알람이 설정되어 있지 않습니다.", memberName)
}

// FormatAlarmList formats user's alarm list
func (f *ResponseFormatter) FormatAlarmList(alarms []AlarmListEntry) string {
	if len(alarms) == 0 {
		return fmt.Sprintf("🔔 설정된 알람이 없습니다.\n\n💡 사용법:\n%s알람 추가 [멤버명]\n예) %s알람 추가 페코라\n예) %s알람 추가 미코",
			f.prefix, f.prefix, f.prefix)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔔 설정된 알람 (%d개)\n\n", len(alarms)))

	for idx, alarm := range alarms {
		sb.WriteString(fmt.Sprintf("%d. %s\n", idx+1, alarm.MemberName))
		if strings.TrimSpace(alarm.NextStream) != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", alarm.NextStream))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("💡 %s알람 제거 [멤버명] 으로 알람 해제", f.prefix))
	return strings.TrimSuffix(sb.String(), "\n")
}

// FormatAlarmCleared formats all alarms cleared confirmation
func (f *ResponseFormatter) FormatAlarmCleared(count int) string {
	if count == 0 {
		return "설정된 알람이 없습니다."
	}
	return fmt.Sprintf("✅ %d개의 알람이 모두 해제되었습니다.", count)
}

// FormatAlarmNotification formats alarm notification message
func (f *ResponseFormatter) FormatAlarmNotification(channel *domain.Channel, stream *domain.Stream, minutesUntil int, users []string) string {
	channelName := channel.GetDisplayName()

	title := util.TruncateString(stream.Title, constants.StringLimits.StreamTitle)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔔 %s 방송 알림\n\n", channelName))

	if minutesUntil > 0 {
		sb.WriteString(fmt.Sprintf("⏰ %d분 후 시작 예정\n", minutesUntil))
	} else {
		sb.WriteString("⏰ 곧 시작합니다!\n")
	}

	sb.WriteString(fmt.Sprintf("📺 %s\n\n", title))
	sb.WriteString(fmt.Sprintf("%s", stream.GetYouTubeURL()))

	if len(users) > 0 {
		sb.WriteString(fmt.Sprintf("\n\n알림 대상: %d명", len(users)))
	}

	return sb.String()
}

// FormatHelp formats help message
func (f *ResponseFormatter) FormatHelp() string {
	p := f.prefix
	return fmt.Sprintf(`🌸 홀로라이브 카카오톡 봇

📺 방송 확인
  %s라이브 - 현재 라이브 중인 방송
  %s라이브 [멤버명] - 특정 멤버 라이브 확인
  %s예정 [시간] - 예정된 방송 (기본 24시간)
  %s멤버 [이름] [일수] - 특정 멤버 일정 (기본 7일)

👤 멤버 정보
  %s정보 [멤버명] - 멤버 프로필 조회
  예: "%s미코 정보", "%s아쿠아에 대해 알려줘"

🔔 알람 설정
  %s알람 추가 [멤버명]
  %s알람 제거 [멤버명]
  %s알람 목록
  %s알람 초기화

💬 자연어 지원
  예: "%s페코라 일정 알려줘", "%s지금 방송하는 사람 있어?"

❤️ Made with love for Hololive fans`, p, p, p, p, p, p, p, p, p, p, p, p, p)
}

// FormatError formats error message
func (f *ResponseFormatter) FormatError(message string) string {
	return fmt.Sprintf("❌ %s", message)
}

// FormatMemberNotFound formats member not found error
func (f *ResponseFormatter) FormatMemberNotFound(memberName string) string {
	return f.FormatError(fmt.Sprintf("'%s' 멤버를 찾을 수 없습니다.", memberName))
}

// FormatTalentProfile formats official talent profile data into a readable message.
func (f *ResponseFormatter) FormatTalentProfile(raw *domain.TalentProfile, translated *domain.TranslatedTalentProfile) string {
	if raw == nil {
		return "❌ 프로필 데이터를 찾을 수 없습니다."
	}

	var sb strings.Builder

	displayName := raw.EnglishName
	if translated != nil && strings.TrimSpace(translated.DisplayName) != "" {
		displayName = translated.DisplayName
	}

	sb.WriteString(fmt.Sprintf("📘 %s", displayName))
	if raw.JapaneseName != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", raw.JapaneseName))
	}
	sb.WriteString("\n")

	catchphrase := strings.TrimSpace(raw.Catchphrase)
	if translated != nil && strings.TrimSpace(translated.Catchphrase) != "" {
		catchphrase = translated.Catchphrase
	}
	if catchphrase != "" {
		sb.WriteString(fmt.Sprintf("🗣️ %s\n", catchphrase))
	}

	summary := ""
	if translated != nil && strings.TrimSpace(translated.Summary) != "" {
		summary = translated.Summary
	} else if raw.Description != "" {
		summary = raw.Description
	}
	if summary != "" {
		sb.WriteString(summary)
		sb.WriteString("\n")
	}

	if translated != nil && len(translated.Highlights) > 0 {
		sb.WriteString("\n✨ 하이라이트\n")
		for _, highlight := range translated.Highlights {
			trimmed := strings.TrimSpace(highlight)
			if trimmed == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("- %s\n", trimmed))
		}
	}

	dataEntries := make([]domain.TranslatedProfileDataRow, 0)
	if translated != nil && len(translated.Data) > 0 {
		dataEntries = translated.Data
	} else {
		for _, entry := range raw.DataEntries {
			if strings.TrimSpace(entry.Label) == "" || strings.TrimSpace(entry.Value) == "" {
				continue
			}
			dataEntries = append(dataEntries, domain.TranslatedProfileDataRow{
				Label: entry.Label,
				Value: entry.Value,
			})
		}
	}

	if len(dataEntries) > 0 {
		sb.WriteString("\n📋 프로필 데이터\n")
		maxRows := len(dataEntries)
		if maxRows > 8 {
			maxRows = 8
		}
		for i := 0; i < maxRows; i++ {
			row := dataEntries[i]
			if strings.TrimSpace(row.Label) == "" || strings.TrimSpace(row.Value) == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", row.Label, row.Value))
		}
	}

	if len(raw.SocialLinks) > 0 {
		sb.WriteString("\n🔗 링크\n")
		maxLinks := len(raw.SocialLinks)
		if maxLinks > 4 {
			maxLinks = 4
		}
		for i := 0; i < maxLinks; i++ {
			link := raw.SocialLinks[i]
			if strings.TrimSpace(link.Label) == "" || strings.TrimSpace(link.URL) == "" {
				continue
			}
			sb.WriteString(fmt.Sprintf("- %s: %s\n", link.Label, link.URL))
		}
	}

	if raw.OfficialURL != "" {
		sb.WriteString("\n🌐 공식 프로필: ")
		sb.WriteString(raw.OfficialURL)
	}

	return strings.TrimSpace(sb.String())
}

// Helper methods

// truncateTitle truncates a title to the maximum length
func (f *ResponseFormatter) truncateTitle(title string) string {
	return util.TruncateString(title, constants.StringLimits.StreamTitle)
}

// formatStreamTimeInfo formats stream time information
func (f *ResponseFormatter) formatStreamTimeInfo(stream *domain.Stream) string {
	if stream == nil || stream.StartScheduled == nil {
		return "시간 미정"
	}

	// Convert to KST
	kstTime := util.FormatKST(*stream.StartScheduled, "01/02 15:04")
	minutesUntil := stream.MinutesUntilStart()

	if minutesUntil <= 0 {
		return kstTime
	}

	hoursUntil := minutesUntil / 60
	minutesRem := minutesUntil % 60

	if hoursUntil > 24 {
		daysUntil := hoursUntil / 24
		return fmt.Sprintf("%s (%d일 후)", kstTime, daysUntil)
	} else if hoursUntil > 0 {
		return fmt.Sprintf("%s (%d시간 %d분 후)", kstTime, hoursUntil, minutesRem)
	} else {
		return fmt.Sprintf("%s (%d분 후)", kstTime, minutesRem)
	}
}
