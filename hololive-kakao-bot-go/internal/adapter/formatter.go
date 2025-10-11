package adapter

import (
	"fmt"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
)

type AlarmListEntry struct {
	MemberName string
	NextStream string
}

type ResponseFormatter struct {
	prefix string
}

type liveStreamView struct {
	ChannelName string
	Title       string
	URL         string
}

type liveStreamsTemplateData struct {
	Count   int
	Streams []liveStreamView
}

type upcomingStreamView struct {
	ChannelName string
	Title       string
	TimeInfo    string
	URL         string
}

type upcomingStreamsTemplateData struct {
	Count   int
	Hours   int
	Streams []upcomingStreamView
}

type scheduleEntryView struct {
	StatusIcon string
	Title      string
	TimeInfo   string
	URL        string
}

type channelScheduleTemplateData struct {
	ChannelName string
	Days        int
	Count       int
	Streams     []scheduleEntryView
}

type alarmAddedTemplateData struct {
	MemberName string
	Added      bool
	NextInfo   string
	Prefix     string
}

type alarmRemovedTemplateData struct {
	MemberName string
	Removed    bool
}

type alarmListTemplateData struct {
	Count  int
	Prefix string
	Alarms []AlarmListEntry
}

type alarmClearedTemplateData struct {
	Count int
}

type alarmNotificationTemplateData struct {
	ChannelName  string
	MinutesUntil int
	Title        string
	URL          string
}

type helpTemplateData struct {
	Prefix string
}

func NewResponseFormatter(prefix string) *ResponseFormatter {
	if strings.TrimSpace(prefix) == "" {
		prefix = "!"
	}
	return &ResponseFormatter{prefix: prefix}
}

func (f *ResponseFormatter) FormatLiveStreams(streams []*domain.Stream) string {

	data := liveStreamsTemplateData{Count: len(streams)}
	if len(streams) > 0 {
		data.Streams = make([]liveStreamView, len(streams))
		for i, stream := range streams {
			data.Streams[i] = liveStreamView{
				ChannelName: stream.ChannelName,
				Title:       f.truncateTitle(stream.Title),
				URL:         stream.GetYouTubeURL(),
			}
		}
	}

	if rendered, err := executeFormatterTemplate("live_streams.tmpl", data); err == nil {
		return rendered
	}

	return f.fallbackLiveStreams(data)
}

func (f *ResponseFormatter) FormatUpcomingStreams(streams []*domain.Stream, hours int) string {

	data := upcomingStreamsTemplateData{Count: len(streams), Hours: hours}
	if len(streams) > 0 {
		data.Streams = make([]upcomingStreamView, len(streams))
		for i, stream := range streams {
			data.Streams[i] = upcomingStreamView{
				ChannelName: stream.ChannelName,
				Title:       f.truncateTitle(stream.Title),
				TimeInfo:    f.formatStreamTimeInfo(stream),
				URL:         stream.GetYouTubeURL(),
			}
		}
	}

	if rendered, err := executeFormatterTemplate("upcoming_streams.tmpl", data); err == nil {
		return rendered
	}

	return f.fallbackUpcomingStreams(data)
}

func (f *ResponseFormatter) FormatChannelSchedule(channel *domain.Channel, streams []*domain.Stream, days int) string {

	data := channelScheduleTemplateData{Days: days, Count: len(streams)}
	if channel != nil {
		data.ChannelName = channel.GetDisplayName()
	}
	if len(streams) > 0 {
		data.Streams = make([]scheduleEntryView, len(streams))
		for i, stream := range streams {
			entry := scheduleEntryView{
				Title: f.truncateTitle(stream.Title),
				URL:   stream.GetYouTubeURL(),
			}

			if stream.IsLive() {
				entry.StatusIcon = "🔴 LIVE"
				entry.TimeInfo = "지금 방송 중"
			} else {
				entry.StatusIcon = "⏰"
				entry.TimeInfo = f.formatStreamTimeInfo(stream)
			}

			data.Streams[i] = entry
		}
	}

	if rendered, err := executeFormatterTemplate("channel_schedule.tmpl", data); err == nil {
		return rendered
	}

	return f.fallbackChannelSchedule(data)
}

func (f *ResponseFormatter) FormatAlarmAdded(memberName string, added bool, nextStreamInfo string) string {

	data := alarmAddedTemplateData{
		MemberName: memberName,
		Added:      added,
		NextInfo:   strings.TrimSpace(nextStreamInfo),
		Prefix:     f.prefix,
	}

	if rendered, err := executeFormatterTemplate("alarm_added.tmpl", data); err == nil {
		return rendered
	}

	return f.fallbackAlarmAdded(data)
}

func (f *ResponseFormatter) FormatAlarmRemoved(memberName string, removed bool) string {

	data := alarmRemovedTemplateData{
		MemberName: memberName,
		Removed:    removed,
	}

	if rendered, err := executeFormatterTemplate("alarm_removed.tmpl", data); err == nil {
		return rendered
	}

	return f.fallbackAlarmRemoved(data)
}

func (f *ResponseFormatter) FormatAlarmList(alarms []AlarmListEntry) string {

	data := alarmListTemplateData{
		Count:  len(alarms),
		Prefix: f.prefix,
		Alarms: alarms,
	}

	if rendered, err := executeFormatterTemplate("alarm_list.tmpl", data); err == nil {
		return rendered
	}

	return f.fallbackAlarmList(data)
}

func (f *ResponseFormatter) FormatAlarmCleared(count int) string {

	data := alarmClearedTemplateData{Count: count}
	if rendered, err := executeFormatterTemplate("alarm_cleared.tmpl", data); err == nil {
		return rendered
	}

	if count == 0 {
		return "설정된 알람이 없습니다."
	}
	return fmt.Sprintf("✅ %d개의 알람이 모두 해제되었습니다.", count)
}

func (f *ResponseFormatter) FormatAlarmNotification(channel *domain.Channel, stream *domain.Stream, minutesUntil int, users []string) string {

	// Users are kept for dispatch logic; count is intentionally excluded from the message.
	_ = users

	channelName := ""
	if channel != nil {
		channelName = channel.GetDisplayName()
	}

	data := alarmNotificationTemplateData{
		ChannelName:  channelName,
		MinutesUntil: minutesUntil,
		Title:        util.TruncateString(stream.Title, constants.StringLimits.StreamTitle),
		URL:          stream.GetYouTubeURL(),
	}

	if rendered, err := executeFormatterTemplate("alarm_notification.tmpl", data); err == nil {
		return rendered
	}

	return f.fallbackAlarmNotification(data)
}

func (f *ResponseFormatter) FormatHelp() string {

	data := helpTemplateData{Prefix: f.prefix}
	if rendered, err := executeFormatterTemplate("help.tmpl", data); err == nil {
		return rendered
	}

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
	
	📊 통계 (NEW!)
	  %s구독자순위 - 지난 7일 구독자 증가 순위 TOP 10
	  자동 알림: 마일스톤 달성 시 (10만, 100만, 500만 등)
	
	💬 자연어 지원
	  예: "%s페코라 일정 알려줘", "%s지금 방송하는 사람 있어?"
	
	❤️ Made with love for Hololive fans`, p, p, p, p, p, p, p, p, p, p, p, p, p, p)
}

func (f *ResponseFormatter) fallbackLiveStreams(data liveStreamsTemplateData) string {
	if data.Count == 0 {
		return "🔴 현재 방송 중인 스트림이 없습니다."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔴 현재 라이브 중 (%d개)\n\n", data.Count))

	for i, stream := range data.Streams {
		if i > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("📺 %s\n", stream.ChannelName))
		sb.WriteString(fmt.Sprintf("   %s\n", stream.Title))
		sb.WriteString(fmt.Sprintf("   %s", stream.URL))

		if i < data.Count-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (f *ResponseFormatter) fallbackUpcomingStreams(data upcomingStreamsTemplateData) string {
	if data.Count == 0 {
		return fmt.Sprintf("📅 %d시간 이내 예정된 방송이 없습니다.", data.Hours)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 예정된 방송 (%d시간 이내, %d개)\n\n", data.Hours, data.Count))

	for i, stream := range data.Streams {
		if i > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("📺 %s\n", stream.ChannelName))
		sb.WriteString(fmt.Sprintf("   %s\n", stream.Title))
		sb.WriteString(fmt.Sprintf("   ⏰ %s\n", stream.TimeInfo))
		sb.WriteString(fmt.Sprintf("   %s", stream.URL))

		if i < data.Count-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (f *ResponseFormatter) fallbackChannelSchedule(data channelScheduleTemplateData) string {
	if data.ChannelName == "" {
		return "❌ 채널 정보를 찾을 수 없습니다."
	}

	if data.Count == 0 {
		return fmt.Sprintf("📅 %s\n%d일 이내 예정된 방송이 없습니다.", data.ChannelName, data.Days)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📅 %s 일정 (%d일 이내, %d개)\n\n", data.ChannelName, data.Days, data.Count))

	for i, stream := range data.Streams {
		if i > 0 {
			sb.WriteString("\n")
		}

		sb.WriteString(fmt.Sprintf("%s %s\n", stream.StatusIcon, stream.Title))
		sb.WriteString(fmt.Sprintf("   %s\n", stream.TimeInfo))
		sb.WriteString(fmt.Sprintf("   %s", stream.URL))

		if i < data.Count-1 {
			sb.WriteString("\n")
		}
	}

	return sb.String()
}

func (f *ResponseFormatter) fallbackAlarmAdded(data alarmAddedTemplateData) string {
	if !data.Added {
		return fmt.Sprintf("ℹ️ %s 알람이 이미 설정되어 있습니다.", data.MemberName)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ %s 알람이 설정되었습니다!\n\n", data.MemberName))
	if data.NextInfo != "" {
		sb.WriteString(data.NextInfo)
		if !strings.HasSuffix(data.NextInfo, "\n") {
			sb.WriteString("\n")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("방송 시작 5분 전에 알림을 받습니다.\n")
	sb.WriteString(fmt.Sprintf("%s알람 목록 으로 확인 가능합니다.", f.prefix))
	return sb.String()
}

func (f *ResponseFormatter) fallbackAlarmRemoved(data alarmRemovedTemplateData) string {
	if data.Removed {
		return fmt.Sprintf("✅ %s 알람이 해제되었습니다.", data.MemberName)
	}
	return fmt.Sprintf("❌ %s 알람이 설정되어 있지 않습니다.", data.MemberName)
}

func (f *ResponseFormatter) fallbackAlarmList(data alarmListTemplateData) string {
	if data.Count == 0 {
		return fmt.Sprintf(`🔔 설정된 알람이 없습니다.

💡 사용법:
%[1]s알람 추가 [멤버명]
예) %[1]s알람 추가 페코라
예) %[1]s알람 추가 미코`, data.Prefix)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔔 설정된 알람 (%d개)\n\n", data.Count))

	for idx, alarm := range data.Alarms {
		sb.WriteString(fmt.Sprintf("%d. %s\n", idx+1, alarm.MemberName))
		if strings.TrimSpace(alarm.NextStream) != "" {
			sb.WriteString(fmt.Sprintf("   %s\n", alarm.NextStream))
		}
		sb.WriteString("\n")
	}

	sb.WriteString(fmt.Sprintf("💡 %s알람 제거 [멤버명] 으로 알람 해제", data.Prefix))
	return strings.TrimSuffix(sb.String(), "\n")
}

func (f *ResponseFormatter) fallbackAlarmNotification(data alarmNotificationTemplateData) string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🔔 %s 방송 알림\n\n", data.ChannelName))

	if data.MinutesUntil > 0 {
		sb.WriteString(fmt.Sprintf("⏰ %d분 후 시작 예정\n", data.MinutesUntil))
	} else {
		sb.WriteString("⏰ 곧 시작합니다!\n")
	}

	sb.WriteString(fmt.Sprintf("\n📺 %s\n\n", data.Title))
	sb.WriteString(data.URL)
	return sb.String()
}

func (f *ResponseFormatter) FormatError(message string) string {
	return fmt.Sprintf("❌ %s", message)
}

func (f *ResponseFormatter) FormatMemberNotFound(memberName string) string {
	return f.FormatError(fmt.Sprintf("'%s' 멤버를 찾을 수 없습니다.", memberName))
}

func (f *ResponseFormatter) FormatTalentProfile(raw *domain.TalentProfile, translated *domain.Translated) string {
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
			translatedLabel := translateSocialLinkLabel(link.Label)
			sb.WriteString(fmt.Sprintf("- %s: %s\n", translatedLabel, link.URL))
		}
	}

	if raw.OfficialURL != "" {
		sb.WriteString("\n🌐 공식 프로필: ")
		sb.WriteString(raw.OfficialURL)
	}

	return strings.TrimSpace(sb.String())
}

func translateSocialLinkLabel(label string) string {
	translations := map[string]string{
		"歌の再生リスト":   "음악 플레이리스트",
		"公式グッズ":     "공식 굿즈",
		"オフィシャルグッズ": "공식 굿즈",
	}

	if korean, ok := translations[label]; ok {
		return korean
	}
	return label
}

func (f *ResponseFormatter) truncateTitle(title string) string {
	return util.TruncateString(title, constants.StringLimits.StreamTitle)
}

func (f *ResponseFormatter) formatStreamTimeInfo(stream *domain.Stream) string {
	if stream == nil || stream.StartScheduled == nil {
		return "시간 미정"
	}

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
