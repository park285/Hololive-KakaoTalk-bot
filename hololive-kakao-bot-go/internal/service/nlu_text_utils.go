package service

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
)

var controlCharsPattern = regexp.MustCompile(`[\x00-\x1F\x7F]`)
var whitespacePattern = regexp.MustCompile(`\s+`)

func sanitizeInput(input string) string {
	withoutControl := controlCharsPattern.ReplaceAllString(input, " ")
	normalized := whitespacePattern.ReplaceAllString(withoutControl, " ")
	trimmed := strings.TrimSpace(normalized)

	if trimmed == "" {
		return ""
	}

	if len(trimmed) > constants.AIInputLimits.MaxQueryLength {
		return trimmed[:constants.AIInputLimits.MaxQueryLength]
	}

	return trimmed
}

func buildClarificationSentence(rawCandidate string) string {
	candidate := strings.TrimSpace(rawCandidate)
	if candidate == "" {
		candidate = "요청"
	}
	escaped := strings.ReplaceAll(candidate, `"`, "'")
	return fmt.Sprintf("누구를 말씀하신 건지 잘 모르겠어요. \"%s\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요.", escaped)
}

func extractQuotedCandidate(message string) string {
	if message == "" {
		return ""
	}
	start := strings.Index(message, "\"")
	if start == -1 {
		return ""
	}
	remaining := message[start+1:]
	end := strings.Index(remaining, "\"")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(remaining[:end])
}
