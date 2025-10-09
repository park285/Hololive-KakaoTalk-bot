package adapter

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/iris"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
)

var controlCharsPattern = regexp.MustCompile(`[\x00-\x1F\x7F]`)

// MessageAdapter converts KakaoTalk messages to bot commands
type MessageAdapter struct {
	prefix string
}

// NewMessageAdapter creates a new MessageAdapter
func NewMessageAdapter(prefix string) *MessageAdapter {
	return &MessageAdapter{prefix: prefix}
}

// ParsedCommand represents a parsed command
type ParsedCommand struct {
	Type       domain.CommandType
	Params     map[string]any
	RawMessage string
}

// ParseMessage parses a KakaoTalk message into a command
func (ma *MessageAdapter) ParseMessage(message *iris.Message) *ParsedCommand {
	if message == nil || message.Msg == "" {
		return ma.createUnknownCommand("")
	}

	text := strings.TrimSpace(message.Msg)

	// Check prefix
	if !strings.HasPrefix(text, ma.prefix) {
		return ma.createUnknownCommand(text)
	}

	// Remove prefix
	commandText := strings.TrimSpace(text[len(ma.prefix):])

	// Split by whitespace
	parts := strings.Fields(commandText)
	if len(parts) == 0 {
		return ma.createUnknownCommand(text)
	}

	command := strings.ToLower(parts[0])
	args := parts[1:]

	// Match commands
	if ma.isLiveCommand(command) {
		return &ParsedCommand{
			Type:       domain.CommandLive,
			Params:     make(map[string]any),
			RawMessage: text,
		}
	}

	if ma.isUpcomingCommand(command) {
		return &ParsedCommand{
			Type:       domain.CommandUpcoming,
			Params:     ma.parseUpcomingArgs(args),
			RawMessage: text,
		}
	}

	if ma.isScheduleCommand(command) {
		return &ParsedCommand{
			Type:       domain.CommandSchedule,
			Params:     ma.parseScheduleArgs(args),
			RawMessage: text,
		}
	}

	if ma.isAlarmCommand(command, args) {
		return ma.parseAlarmCommand(command, args, text)
	}

	if ma.isHelpCommand(command) {
		return &ParsedCommand{
			Type:       domain.CommandHelp,
			Params:     make(map[string]any),
			RawMessage: text,
		}
	}

	if ma.isMemberInfoCommand(command) {
		query := strings.TrimSpace(strings.Join(args, " "))
		params := make(map[string]any)
		if query != "" {
			params["query"] = query
		}
		return &ParsedCommand{
			Type:       domain.CommandMemberInfo,
			Params:     params,
			RawMessage: text,
		}
	}

	if ma.isAskCommand(command) {
		question := ma.sanitizeForGemini(strings.Join(args, " "))
		if question == "" {
			return ma.createUnknownCommand(text)
		}
		return &ParsedCommand{
			Type:       domain.CommandAsk,
			Params:     map[string]any{"question": question},
			RawMessage: text,
		}
	}

	// AI natural language processing
	sanitized := ma.sanitizeForGemini(commandText)
	if sanitized == "" {
		return ma.createUnknownCommand(text)
	}

	return &ParsedCommand{
		Type:       domain.CommandAsk,
		Params:     map[string]any{"question": sanitized},
		RawMessage: text,
	}
}

// Command matchers

func (ma *MessageAdapter) isLiveCommand(cmd string) bool {
	return util.Contains([]string{"라이브", "live", "방송중", "생방송"}, cmd)
}

func (ma *MessageAdapter) isUpcomingCommand(cmd string) bool {
	return util.Contains([]string{"예정", "upcoming"}, cmd)
}

func (ma *MessageAdapter) isScheduleCommand(cmd string) bool {
	return util.Contains([]string{"일정", "스케줄", "schedule", "멤버", "member"}, cmd)
}

func (ma *MessageAdapter) isAlarmCommand(cmd string, args []string) bool {
	if util.Contains([]string{"알람", "알림", "alarm"}, cmd) {
		return true
	}

	if len(args) > 0 {
		subCmd := strings.ToLower(args[0])
		return util.Contains([]string{"추가", "set", "add", "설정", "제거", "remove", "del", "삭제", "목록", "list", "초기화", "clear"}, subCmd)
	}

	return false
}

func (ma *MessageAdapter) isHelpCommand(cmd string) bool {
	return util.Contains([]string{"도움말", "도움", "help", "명령어", "commands"}, cmd)
}

func (ma *MessageAdapter) isAskCommand(cmd string) bool {
	return util.Contains([]string{"질문", "ask"}, cmd)
}

func (ma *MessageAdapter) isMemberInfoCommand(cmd string) bool {
	return util.Contains([]string{"멤버", "member", "프로필", "profile", "정보", "info"}, cmd)
}

// Argument parsers

func (ma *MessageAdapter) parseUpcomingArgs(args []string) map[string]any {
	if len(args) == 0 {
		return map[string]any{"hours": 24}
	}

	hours, err := strconv.Atoi(args[0])
	if err != nil {
		return map[string]any{"hours": 24}
	}

	// Clamp to 1-168
	if hours < 1 {
		hours = 1
	}
	if hours > 168 {
		hours = 168
	}

	return map[string]any{"hours": hours}
}

func (ma *MessageAdapter) parseScheduleArgs(args []string) map[string]any {
	if len(args) == 0 {
		return make(map[string]any)
	}

	member := args[0]
	days := 7

	if len(args) > 1 {
		if d, err := strconv.Atoi(args[1]); err == nil {
			days = d
			if days < 1 {
				days = 1
			}
			if days > 30 {
				days = 30
			}
		}
	}

	return map[string]any{
		"member": member,
		"days":   days,
	}
}

func (ma *MessageAdapter) parseAlarmCommand(cmd string, args []string, rawMessage string) *ParsedCommand {
	if len(args) == 0 {
		return &ParsedCommand{
			Type:       domain.CommandAlarmList,
			Params:     map[string]any{"action": "list"},
			RawMessage: rawMessage,
		}
	}

	subCmd := strings.ToLower(args[0])
	restArgs := args[1:]

	// Add/Set
	if util.Contains([]string{"추가", "설정", "set", "add"}, subCmd) {
		return &ParsedCommand{
			Type: domain.CommandAlarmAdd,
			Params: map[string]any{
				"action": "add",
				"member": strings.Join(restArgs, " "),
			},
			RawMessage: rawMessage,
		}
	}

	// Remove/Delete
	if util.Contains([]string{"제거", "삭제", "remove", "del", "delete"}, subCmd) {
		return &ParsedCommand{
			Type: domain.CommandAlarmRemove,
			Params: map[string]any{
				"action": "remove",
				"member": strings.Join(restArgs, " "),
			},
			RawMessage: rawMessage,
		}
	}

	// List
	if util.Contains([]string{"목록", "list", "show"}, subCmd) {
		return &ParsedCommand{
			Type:       domain.CommandAlarmList,
			Params:     map[string]any{"action": "list"},
			RawMessage: rawMessage,
		}
	}

	// Clear
	if util.Contains([]string{"초기화", "clear", "reset"}, subCmd) {
		return &ParsedCommand{
			Type:       domain.CommandAlarmClear,
			Params:     map[string]any{"action": "clear"},
			RawMessage: rawMessage,
		}
	}

	// Default to help
	return &ParsedCommand{
		Type:       domain.CommandHelp,
		Params:     make(map[string]any),
		RawMessage: rawMessage,
	}
}

func (ma *MessageAdapter) createUnknownCommand(text string) *ParsedCommand {
	return &ParsedCommand{
		Type:       domain.CommandUnknown,
		Params:     make(map[string]any),
		RawMessage: text,
	}
}

func (ma *MessageAdapter) sanitizeForGemini(input string) string {
	// Remove control characters
	withoutControl := controlCharsPattern.ReplaceAllString(input, " ")

	// Normalize whitespace
	normalized := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(withoutControl, " "))

	if len(normalized) == 0 {
		return ""
	}

	if len(normalized) > constants.AIInputLimits.MaxQueryLength {
		return normalized[:constants.AIInputLimits.MaxQueryLength]
	}

	return normalized
}

