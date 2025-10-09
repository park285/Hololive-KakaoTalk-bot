package domain

import "time"

type CommandContext struct {
	Room        string
	RoomName    string
	Sender      string
	IsGroupChat bool
	Message     string
	Timestamp   time.Time
}

func NewCommandContext(room, roomName, sender, message string, isGroupChat bool) *CommandContext {
	return &CommandContext{
		Room:        room,
		RoomName:    roomName,
		Sender:      sender,
		IsGroupChat: isGroupChat,
		Message:     message,
		Timestamp:   time.Now(),
	}
}
