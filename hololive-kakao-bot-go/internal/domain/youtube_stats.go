package domain

import "time"

type TimestampedStats struct {
	ChannelID       string    `json:"channel_id"`
	MemberName      string    `json:"member_name"`
	SubscriberCount uint64    `json:"subscriber_count"`
	VideoCount      uint64    `json:"video_count"`
	ViewCount       uint64    `json:"view_count"`
	Timestamp       time.Time `json:"timestamp"`
}

type MilestoneType string

const (
	MilestoneSubscribers MilestoneType = "subscribers"
	MilestoneVideos      MilestoneType = "videos"
	MilestoneViews       MilestoneType = "views"
)

type Milestone struct {
	ChannelID  string        `json:"channel_id"`
	MemberName string        `json:"member_name"`
	Type       MilestoneType `json:"type"`
	Value      uint64        `json:"value"` // e.g., 1000000 for 1M subscribers
	AchievedAt time.Time     `json:"achieved_at"`
	Notified   bool          `json:"notified"`
}

type StatsChange struct {
	ChannelID        string            `json:"channel_id"`
	MemberName       string            `json:"member_name"`
	SubscriberChange int64             `json:"subscriber_change"`
	VideoChange      int64             `json:"video_change"`
	ViewChange       int64             `json:"view_change"`
	PreviousStats    *TimestampedStats `json:"previous_stats"`
	CurrentStats     *TimestampedStats `json:"current_stats"`
	DetectedAt       time.Time         `json:"detected_at"`
}

type DailySummary struct {
	Date               time.Time   `json:"date"`
	TotalChanges       int         `json:"total_changes"`
	MilestonesAchieved int         `json:"milestones_achieved"`
	NewVideosDetected  int         `json:"new_videos_detected"`
	TopGainers         []RankEntry `json:"top_gainers"`
	TopUploaders       []RankEntry `json:"top_uploaders"`
}

type RankEntry struct {
	ChannelID  string `json:"channel_id"`
	MemberName string `json:"member_name"`
	Value      int64  `json:"value"` // subscriber change or video count
	Rank       int    `json:"rank"`
}

type TrendData struct {
	ChannelID        string    `json:"channel_id"`
	MemberName       string    `json:"member_name"`
	Period           string    `json:"period"` // "daily", "weekly", "monthly"
	SubscriberGrowth int64     `json:"subscriber_growth"`
	VideoUploadRate  float64   `json:"video_upload_rate"` // videos per day
	AvgViewsPerVideo uint64    `json:"avg_views_per_video"`
	UpdatedAt        time.Time `json:"updated_at"`
}
