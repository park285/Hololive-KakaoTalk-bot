package prompt

import (
	"fmt"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

// CandidateChannel represents a channel candidate for selection
type CandidateChannel struct {
	Index       int
	Name        string
	EnglishName string
	ID          string
}

// SelectorPromptVars holds variables for the selector prompt template
type SelectorPromptVars struct {
	UserQuery          string
	CandidateChannels  []CandidateChannel
}

// BuildSelectorPrompt builds the Gemini channel selector prompt
func BuildSelectorPrompt(userQuery string, channels []*domain.Channel) string {
	candidates := make([]string, len(channels))
	for i, ch := range channels {
		englishName := "N/A"
		if ch.EnglishName != nil {
			englishName = *ch.EnglishName
		}
		candidates[i] = fmt.Sprintf("%d. %s (English: %s, ID: %s)",
			i, ch.Name, englishName, ch.ID)
	}

	channelList := strings.Join(candidates, "\n")

	return fmt.Sprintf(`You are a VTuber channel matcher for Hololive.

**User Query:** "%s"

**Candidate Channels:**
%s

**Task:** Select the channel that BEST matches the user query.

**Output JSON Format:**
{
  "selectedIndex": <number, 0-based index of best match, or -1 if no good match>,
  "confidence": <number, 0.0 to 1.0>,
  "reasoning": "<brief explanation in Korean>"
}

**Matching Priority:**
1. Exact name match (any language)
2. Exact ID match
3. Partial name match (start with)
4. If confidence < 0.7, return selectedIndex: -1`,
		userQuery,
		channelList,
	)
}
