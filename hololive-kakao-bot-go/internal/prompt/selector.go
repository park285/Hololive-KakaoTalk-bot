package prompt

import "github.com/kapu/hololive-kakao-bot-go/internal/domain"

func BuildSelector(userQuery string, channels []*domain.Channel) string {
	data := ChannelSelectorData{
		UserQuery:         userQuery,
		CandidateChannels: make([]ChannelCandidate, len(channels)),
	}

	for i, ch := range channels {
		englishName := "N/A"
		if ch.EnglishName != nil {
			englishName = *ch.EnglishName
		}
		data.CandidateChannels[i] = ChannelCandidate{
			Index:       i,
			Name:        ch.Name,
			EnglishName: englishName,
			ID:          ch.ID,
		}
	}

	text, err := DefaultPromptBuilder().Render(TemplateChannelSelector, data)
	if err != nil {
		return ""
	}
	return text
}
