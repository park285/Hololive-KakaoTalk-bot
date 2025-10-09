package domain

import "strings"

// MemberIntentType represents the high-level intent classification for a natural language question.
type MemberIntentType string

const (
	// MemberIntentUnknown indicates the model could not classify the intent.
	MemberIntentUnknown MemberIntentType = "unknown"
	// MemberIntentMemberInfo indicates the user is asking for member information or profile.
	MemberIntentMemberInfo MemberIntentType = "member_info"
	// MemberIntentOther indicates the user intent is not related to member information.
	MemberIntentOther MemberIntentType = "other"
)

// MemberIntentClassification captures the intent prediction from the LLM classifier.
type MemberIntentClassification struct {
	Intent     MemberIntentType `json:"intent"`
	Confidence float64          `json:"confidence"`
	Reasoning  string           `json:"reasoning"`
}

// Normalize maps free-form intent strings into known intent types.
func NormalizeMemberIntent(raw string) MemberIntentType {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(MemberIntentMemberInfo):
		return MemberIntentMemberInfo
	case string(MemberIntentOther):
		return MemberIntentOther
	default:
		return MemberIntentUnknown
	}
}

// IsMemberInfoIntent reports whether the classification indicates a member info request with workable confidence.
func (mic *MemberIntentClassification) IsMemberInfoIntent() bool {
	if mic == nil {
		return false
	}
	intent := NormalizeMemberIntent(string(mic.Intent))
	if intent != MemberIntentMemberInfo {
		return false
	}
	// Accept modest confidence so that ambiguous "알려줄래" 류 표현도 통과하도록 함.
	return mic.Confidence >= 0.35
}
