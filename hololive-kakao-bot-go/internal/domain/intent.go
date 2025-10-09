package domain

import (
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
)

type MemberIntentType string

const (
	MemberIntentUnknown MemberIntentType = "unknown"
	MemberIntentMemberInfo MemberIntentType = "member_info"
	MemberIntentOther MemberIntentType = "other"
)

type MemberIntent struct {
	Intent     MemberIntentType `json:"intent"`
	Confidence float64          `json:"confidence"`
	Reasoning  string           `json:"reasoning"`
}

func NormalizeMemberIntent(raw string) MemberIntentType {
	switch util.Normalize(raw) {
	case string(MemberIntentMemberInfo):
		return MemberIntentMemberInfo
	case string(MemberIntentOther):
		return MemberIntentOther
	default:
		return MemberIntentUnknown
	}
}

func (mic *MemberIntent) IsMemberInfoIntent() bool {
	if mic == nil {
		return false
	}
	intent := NormalizeMemberIntent(string(mic.Intent))
	if intent != MemberIntentMemberInfo {
		return false
	}
	return mic.Confidence >= 0.35
}
