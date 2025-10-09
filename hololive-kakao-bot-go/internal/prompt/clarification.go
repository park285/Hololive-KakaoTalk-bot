package prompt

import (
	"fmt"
	"strings"
)

func BuildBasic(userQuery string) string {
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
`, userQuery)
}

func BuildWithMembers(userQuery string, memberNames []string) string {
	memberList := ""
	if len(memberNames) > 0 {
		memberList = fmt.Sprintf("\n\n홀로라이브 멤버 리스트: [%s]", strings.Join(memberNames, ", "))
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
`, userQuery, memberList)
}

func BuildWithoutMembers(userQuery string) string {
	return fmt.Sprintf(`User question: "%s"

Analyze using the Hololive member list above.

Return JSON:
{
  "is_hololive_related": true or false,
  "message": "Korean clarification message if hololive-related, empty otherwise",
  "candidate": "member name if 80%%%%+ confident, empty otherwise"
}

Rules:
- ALL messages MUST be in Korean (절대 영어 금지)
- is_hololive_related=true if:
  * Asking about any person (even if not in member list)
  * About Hololive content/schedules
- If person NOT in list:
  * is_hololive_related=true
  * message: "누구를 말씀하신 건지 잘 모르겠어요. \"<person>\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요."
- Candidate only if 80%%%%+ confident, else empty
- Focus on CORE INTENT, ignore all grammar/conjugation differences
`, userQuery)
}
