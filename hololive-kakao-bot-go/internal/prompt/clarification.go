package prompt

import "strings"

func BuildBasic(userQuery string) string {
	data := ClarificationBasicData{
		UserQuery: userQuery,
	}
	text, err := DefaultPromptBuilder().Render(TemplateClarificationBasic, data)
	if err != nil {
		return ""
	}
	return text
}

func BuildWithMembers(userQuery string, memberNames []string) string {
	data := ClarificationWithMembersData{
		UserQuery:  userQuery,
		MemberList: strings.Join(memberNames, ", "),
	}
	text, err := DefaultPromptBuilder().Render(TemplateClarificationWithMembers, data)
	if err != nil {
		return ""
	}
	return text
}

func BuildWithoutMembers(userQuery string) string {
	data := ClarificationWithoutMembersData{
		UserQuery: userQuery,
	}
	text, err := DefaultPromptBuilder().Render(TemplateClarificationWithoutMembers, data)
	if err != nil {
		return ""
	}
	return text
}
