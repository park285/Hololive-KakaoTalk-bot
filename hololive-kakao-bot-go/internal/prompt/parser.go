package prompt

import "fmt"

// ParserPromptVars holds variables for the parser prompt template
type ParserPromptVars struct {
	MemberCount       int
	MemberListWithIDs string
	UserQuery         string
}

// BuildParserPrompt builds the Gemini natural language parser prompt
func BuildParserPrompt(vars ParserPromptVars) string {
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
- Detect "누구", "who", "소개", "profile", "정보" questions about a member → member_info`,
		vars.MemberCount,
		vars.MemberListWithIDs,
		vars.UserQuery,
	)
}
