package domain

import "time"

type StreamStatus string

const (
	StreamStatusLive     StreamStatus = "live"
	StreamStatusUpcoming StreamStatus = "upcoming"
	StreamStatusPast     StreamStatus = "past"
)

func (s StreamStatus) String() string {
	return string(s)
}

func (s StreamStatus) IsValid() bool {
	switch s {
	case StreamStatusLive, StreamStatusUpcoming, StreamStatusPast:
		return true
	default:
		return false
	}
}

type Stream struct {
	ID             string       `json:"id"`
	Title          string       `json:"title"`
	ChannelID      string       `json:"channel_id"`
	ChannelName    string       `json:"channel_name"`
	Status         StreamStatus `json:"status"`
	StartScheduled *time.Time   `json:"start_scheduled,omitempty"`
	StartActual    *time.Time   `json:"start_actual,omitempty"`
	Duration       *int         `json:"duration,omitempty"` // seconds
	Thumbnail      *string      `json:"thumbnail,omitempty"`
	Link           *string      `json:"link,omitempty"`
	TopicID        *string      `json:"topic_id,omitempty"`
	Channel        *Channel     `json:"channel,omitempty"`
}

func (s *Stream) IsLive() bool {
	if s == nil {
		return false
	}
	return s.Status == StreamStatusLive
}

func (s *Stream) IsUpcoming() bool {
	if s == nil {
		return false
	}
	return s.Status == StreamStatusUpcoming
}

func (s *Stream) IsPast() bool {
	if s == nil {
		return false
	}
	return s.Status == StreamStatusPast
}

func (s *Stream) GetYouTubeURL() string {
	if s == nil {
		return ""
	}
	if s.Link != nil && *s.Link != "" {
		return *s.Link
	}
	return "https://youtube.com/watch?v=" + s.ID
}

func (s *Stream) TimeUntilStart() *time.Duration {
	if s.StartScheduled == nil {
		return nil
	}

	now := time.Now()
	if s.StartScheduled.Before(now) {
		return nil
	}

	duration := s.StartScheduled.Sub(now)
	return &duration
}

func (s *Stream) MinutesUntilStart() int {
	duration := s.TimeUntilStart()
	if duration == nil {
		return -1
	}
	return int(duration.Minutes())
}
