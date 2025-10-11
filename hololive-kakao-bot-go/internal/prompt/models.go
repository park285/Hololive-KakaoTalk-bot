package prompt

type ParserPromptData struct {
	MemberCount       int
	MemberListWithIDs string
	UserQuery         string
}

type ClarificationBasicData struct {
	UserQuery string
}

type ClarificationWithMembersData struct {
	UserQuery  string
	MemberList string
}

type ClarificationWithoutMembersData struct {
	UserQuery string
}

type ChannelCandidate struct {
	Index       int
	Name        string
	EnglishName string
	ID          string
}

type ChannelSelectorData struct {
	UserQuery         string
	CandidateChannels []ChannelCandidate
}
