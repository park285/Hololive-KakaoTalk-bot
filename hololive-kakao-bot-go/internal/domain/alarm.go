package domain

import "time"

type Alarm struct {
	RoomID     string    `json:"room_id"`     // KakaoTalk room ID
	UserID     string    `json:"user_id"`     // KakaoTalk user ID
	ChannelID  string    `json:"channel_id"`  // YouTube channel ID
	MemberName string    `json:"member_name"` // Member name for display
	CreatedAt  time.Time `json:"created_at"`
}

func NewAlarm(roomID, userID, channelID, memberName string) *Alarm {
	return &Alarm{
		RoomID:     roomID,
		UserID:     userID,
		ChannelID:  channelID,
		MemberName: memberName,
		CreatedAt:  time.Now(),
	}
}

type AlarmNotification struct {
	RoomID       string   `json:"room_id"`
	Channel      *Channel `json:"channel"`
	Stream       *Stream  `json:"stream"`
	MinutesUntil int      `json:"minutes_until"`
	Users        []string `json:"users"`
}

func NewAlarmNotification(roomID string, channel *Channel, stream *Stream, minutesUntil int, users []string) *AlarmNotification {
	return &AlarmNotification{
		RoomID:       roomID,
		Channel:      channel,
		Stream:       stream,
		MinutesUntil: minutesUntil,
		Users:        users,
	}
}

func (n *AlarmNotification) UserCount() int {
	return len(n.Users)
}
