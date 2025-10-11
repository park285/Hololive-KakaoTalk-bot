package prompt

type ParserPromptVars struct {
	MemberCount       int
	MemberListWithIDs string
	UserQuery         string
}

func BuildParserPrompt(vars ParserPromptVars) string {
	data := ParserPromptData{
		MemberCount:       vars.MemberCount,
		MemberListWithIDs: vars.MemberListWithIDs,
		UserQuery:         vars.UserQuery,
	}
	text, err := DefaultPromptBuilder().Render(TemplateParserPrompt, data)
	if err != nil {
		return ""
	}
	return text
}
