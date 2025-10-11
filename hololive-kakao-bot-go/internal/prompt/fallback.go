package prompt

import (
	"fmt"
	"strings"
)

func FallbackParserPrompt(data ParserPromptData) string {
	return fmt.Sprintf(`You are a natural language parser for a Hololive Discord bot.
Parse user's query (Korean/English/Japanese) and convert it to appropriate Discord command.

## Hololive Members Database (Total: %d members):
**Format**: EnglishName|KoreanAliases,JapaneseAliases|ChannelID
**Examples**:
  - "Tsunomaki Watame|양이모,와타메,와타오지,Watame Ch. 角巻わため|UCqm3BQLlJfvkTsX_hvm0UmA"
  - "Nakiri Ayame|아야메,오니,Nakiri Ayame Ch. 百鬼あやめ|UC7fk0CB07ly8oSl0aqKkqFg"

**Database**:
%s

## Available Commands:

1. **live** [member] - Currently live streams
   - With member: Check if specific member is live (e.g., "로보코 방송중?")
   - Without member: All live streams (e.g., "지금 방송")
2. **upcoming** [hours=24] - Upcoming streams (e.g., "오늘 일정")
3. **schedule** <member> [days=7] - Member schedule (e.g., "페코라 7일")
4. **member_info** <member> - Show official profile summary (e.g., "마린 누구야")
5. **alarm_list** - Show my alarms (e.g., "내 알람", "알람 목록")
6. **alarm_add** <member> - Add alarm (e.g., "미코 알람 추가")
7. **alarm_remove** <member> - Remove alarm (e.g., "페코라 알람 제거")
8. **help** - Help message (e.g., "도움말")
9. **unknown** - Cannot determine

## User Query:
"%s"

## Response Format (JSON ONLY):

**Single Command**:
{
  "command": "live|upcoming|schedule|member_info|alarm_list|alarm_add|alarm_remove|help|unknown",
  "params": {
    "member": "Exact English name from database (for schedule/alarm)",
    "channel_id": "Channel ID from database (for schedule/alarm, MUST match exactly)",
    "hours": number (for upcoming),
    "days": number (for schedule)
  },
  "confidence": 0.0-1.0,
  "reasoning": "Brief explanation (max 10 words)"
}

**Multiple Commands** (when user requests multiple actions):
[
  {
    "command": "schedule",
    "params": {"member": "...", "channel_id": "...", "days": 7},
    "confidence": 0.95,
    "reasoning": "Check schedule first"
  },
  {
    "command": "alarm_add",
    "params": {"member": "...", "channel_id": "..."},
    "confidence": 0.95,
    "reasoning": "Then set alarm"
  }
]

**Rules**:
- **Member Matching**: Match user's input against BOTH English name AND aliases (Korean/Japanese)
  - Example: "양이모" → "Tsunomaki Watame" (matched via Korean alias)
  - Example: "아야메" → "Nakiri Ayame" (matched via Korean alias)
- Use exact English name from database for "member" param
- Use exact Channel ID from database for "channel_id" param
- Confidence: >0.9 (certain), 0.5-0.9 (moderate), <0.5 (uncertain)
- If member not found in database or aliases, return command: "unknown"
- Detect alarm intent ("알람", "구독", "알림", "설정", "알려줘", "notification", "subscribe") → alarm_add/alarm_remove
- Detect "누구", "who", "소개", "profile", "정보" questions about a member → member_info`, data.MemberCount, data.MemberListWithIDs, data.UserQuery)
}

func FallbackClarificationBasic(data ClarificationBasicData) string {
	return fmt.Sprintf(`You are a Hololive assistant who clarifies ambiguous user requests in Korean.

The user asked: "%s"

Return JSON that follows this structure exactly:
{
  "candidate": "<best-guess Hololive member name ONLY if you are confident (80%% or higher certainty); otherwise leave completely empty>",
  "message": "누구를 말씀하신 건지 잘 모르겠어요. \"<candidate-or-original-query>\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요."
}

CRITICAL Guidelines:
- ONLY fill "candidate" if you are HIGHLY CONFIDENT (80%% certainty or more) about which Hololive member is being referenced
- If you have ANY doubt or uncertainty, leave "candidate" as an empty string: ""
- DO NOT guess or use a default member when uncertain
- DO NOT use the first member in any list as a fallback
- Replace <candidate-or-original-query> with the candidate name if confident, otherwise use the user's original wording
- Do not add any extra text, punctuation, emojis, or formatting outside the sentence above
- Keep everything in a single sentence as shown
`, data.UserQuery)
}

func FallbackClarificationWithMembers(data ClarificationWithMembersData) string {
	memberList := ""
	if strings.TrimSpace(data.MemberList) != "" {
		memberList = fmt.Sprintf("\n\n홀로라이브 멤버 리스트: [%s]", data.MemberList)
	}

	return fmt.Sprintf(`당신은 홀로라이브 관련 질문을 판별하고 한국어로 명확화 메시지를 생성하는 도우미입니다.

사용자 질문: "%s"%s

분석 원칙:
1. 질문의 **핵심 의도**만 파악하세요 (표현 방식, 문법, 어미는 무시)
2. 언급된 인물이 홀로라이브 멤버 리스트에 있는지 확인
3. 동사의 활용형 차이는 완전히 무시 (예: "알리다"의 모든 변형은 동일한 의도)
4. 존댓말/반말, 의문형/평서형 차이도 무시하고 의도만 판단

JSON 형식으로 반환:
{
  "is_hololive_related": true 또는 false,
  "message": "홀로라이브 관련이면 한국어 명확화 메시지, 아니면 빈 문자열",
  "candidate": "80%% 이상 확신하는 멤버 이름, 불확실하면 빈 문자열"
}

필수 규칙:
- 모든 응답은 반드시 한국어로 작성 (절대 영어 사용 금지)
- is_hololive_related=true 조건:
  * 사람에 대한 정보를 요청하는 질문 (멤버 리스트에 없어도 true)
  * 홀로라이브 콘텐츠, 일정, 활동에 대한 질문
- 멤버 리스트에 없는 인물 언급 시:
  * is_hololive_related=true
  * message: "누구를 말씀하신 건지 잘 모르겠어요. \"<인물명>\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요."
- candidate는 80%% 이상 확신할 때만, 불확실하면 빈 문자열
- 절대 기본 멤버를 추측하거나 첫 번째 멤버를 사용하지 말 것
- message는 정중하고 자연스러운 한국어로 작성
- 질문의 본질적 의도만 판단하고, 표현의 미묘한 차이는 모두 무시하세요
`, data.UserQuery, memberList)
}

func FallbackClarificationWithoutMembers(data ClarificationWithoutMembersData) string {
	return fmt.Sprintf(`User question: "%s"

Analyze using the Hololive member list above.

Return JSON:
{
  "is_hololive_related": true or false,
  "message": "Korean clarification message if hololive-related, empty otherwise",
  "candidate": "member name if 80%%+ confident, empty otherwise"
}

Rules:
- ALL messages MUST be in Korean (절대 영어 금지)
- is_hololive_related=true if:
  * Asking about any person (even if not in member list)
  * About Hololive content/schedules
- If person NOT in list:
  * is_hololive_related=true
  * message: "누구를 말씀하신 건지 잘 모르겠어요. \"<person>\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요."
- Candidate only if 80%%+ confident, else empty
- Focus on CORE INTENT, ignore all grammar/conjugation differences
`, data.UserQuery)
}

func FallbackChannelSelector(data ChannelSelectorData) string {
	builder := &strings.Builder{}
	builder.WriteString("You are a VTuber channel matcher for Hololive.\n\n")
	builder.WriteString(fmt.Sprintf("**User Query:** \"%s\"\n\n", data.UserQuery))
	builder.WriteString("**Candidate Channels:**\n")
	for _, ch := range data.CandidateChannels {
		builder.WriteString(fmt.Sprintf("%d. %s (English: %s, ID: %s)\n", ch.Index, ch.Name, ch.EnglishName, ch.ID))
	}
	builder.WriteString(`
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
4. If confidence < 0.7, return selectedIndex: -1`)

	return builder.String()
}
