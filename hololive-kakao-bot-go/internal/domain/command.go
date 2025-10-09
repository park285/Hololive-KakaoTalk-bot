package domain

type CommandType string

const (
	CommandLive        CommandType = "live"
	CommandUpcoming    CommandType = "upcoming"
	CommandSchedule    CommandType = "schedule"
	CommandHelp        CommandType = "help"
	CommandAlarmAdd    CommandType = "alarm_add"
	CommandAlarmRemove CommandType = "alarm_remove"
	CommandAlarmList   CommandType = "alarm_list"
	CommandAlarmClear  CommandType = "alarm_clear"
	CommandAsk         CommandType = "ask"
	CommandMemberInfo  CommandType = "member_info"
	CommandStats       CommandType = "stats"
	CommandUnknown     CommandType = "unknown"
)

func (c CommandType) String() string {
	return string(c)
}

func (c CommandType) IsValid() bool {
	switch c {
	case CommandLive, CommandUpcoming, CommandSchedule, CommandHelp,
		CommandAlarmAdd, CommandAlarmRemove, CommandAlarmList, CommandAlarmClear,
		CommandAsk, CommandMemberInfo, CommandUnknown:
		return true
	default:
		return false
	}
}

type ParseResult struct {
	Command    CommandType    `json:"command"`
	Params     map[string]any `json:"params"`
	Confidence float64        `json:"confidence"`
	Reasoning  string         `json:"reasoning"`
}

type ParseResults struct {
	Single   *ParseResult
	Multiple []*ParseResult
}

type ChannelSelection struct {
	SelectedIndex int     `json:"selectedIndex"`
	Confidence    float64 `json:"confidence"`
	Reasoning     string  `json:"reasoning"`
}

func (pr *ParseResults) IsSingle() bool {
	return pr.Single != nil
}

func (pr *ParseResults) IsMultiple() bool {
	return len(pr.Multiple) > 0
}

func (pr *ParseResults) GetCommands() []*ParseResult {
	if pr.IsSingle() {
		return []*ParseResult{pr.Single}
	}
	return pr.Multiple
}
