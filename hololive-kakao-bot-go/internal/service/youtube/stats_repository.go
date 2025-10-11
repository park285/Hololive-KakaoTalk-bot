package youtube

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/database"
	"go.uber.org/zap"
)

type YouTubeStatsRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewYouTubeStatsRepository(postgres *database.PostgresService, logger *zap.Logger) *YouTubeStatsRepository {
	return &YouTubeStatsRepository{
		db:     postgres.GetDB(),
		logger: logger,
	}
}

func (r *YouTubeStatsRepository) SaveStats(ctx context.Context, stats *domain.TimestampedStats) error {
	query := `
		INSERT INTO youtube_stats_history (time, channel_id, member_name, subscribers, videos, views)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (time, channel_id) DO UPDATE
		SET subscribers = EXCLUDED.subscribers,
		    videos = EXCLUDED.videos,
		    views = EXCLUDED.views
	`

	_, err := r.db.ExecContext(ctx, query,
		stats.Timestamp,
		stats.ChannelID,
		stats.MemberName,
		stats.SubscriberCount,
		stats.VideoCount,
		stats.ViewCount,
	)

	if err != nil {
		return fmt.Errorf("failed to save stats: %w", err)
	}

	r.logger.Debug("Stats saved to TimescaleDB",
		zap.String("channel", stats.ChannelID),
		zap.Uint64("subscribers", stats.SubscriberCount),
	)

	return nil
}

func (r *YouTubeStatsRepository) GetLatestStats(ctx context.Context, channelID string) (*domain.TimestampedStats, error) {
	query := `
		SELECT time, channel_id, member_name, subscribers, videos, views
		FROM youtube_stats_history
		WHERE channel_id = $1
		ORDER BY time DESC
		LIMIT 1
	`

	var stats domain.TimestampedStats
	var memberName sql.NullString

	err := r.db.QueryRowContext(ctx, query, channelID).Scan(
		&stats.Timestamp,
		&stats.ChannelID,
		&memberName,
		&stats.SubscriberCount,
		&stats.VideoCount,
		&stats.ViewCount,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest stats: %w", err)
	}

	if memberName.Valid {
		stats.MemberName = memberName.String
	}

	return &stats, nil
}

func (r *YouTubeStatsRepository) RecordChange(ctx context.Context, change *domain.StatsChange) error {
	query := `
		INSERT INTO youtube_stats_changes
		(channel_id, member_name, subscriber_change, video_change, view_change,
		 previous_subs, current_subs, previous_videos, current_videos, detected_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`

	var prevSubs, currSubs, prevVideos, currVideos sql.NullInt64

	if change.PreviousStats != nil {
		prevSubs = sql.NullInt64{Int64: int64(change.PreviousStats.SubscriberCount), Valid: true}
		prevVideos = sql.NullInt64{Int64: int64(change.PreviousStats.VideoCount), Valid: true}
	}

	if change.CurrentStats != nil {
		currSubs = sql.NullInt64{Int64: int64(change.CurrentStats.SubscriberCount), Valid: true}
		currVideos = sql.NullInt64{Int64: int64(change.CurrentStats.VideoCount), Valid: true}
	}

	_, err := r.db.ExecContext(ctx, query,
		change.ChannelID,
		change.MemberName,
		change.SubscriberChange,
		change.VideoChange,
		change.ViewChange,
		prevSubs,
		currSubs,
		prevVideos,
		currVideos,
		change.DetectedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to record change: %w", err)
	}

	r.logger.Info("Change recorded",
		zap.String("member", change.MemberName),
		zap.Int64("sub_change", change.SubscriberChange),
		zap.Int64("vid_change", change.VideoChange),
	)

	return nil
}

func (r *YouTubeStatsRepository) SaveMilestone(ctx context.Context, milestone *domain.Milestone) error {
	query := `
		INSERT INTO youtube_milestones (channel_id, member_name, type, value, achieved_at, notified)
		VALUES ($1, $2, $3, $4, $5, $6)
	`

	_, err := r.db.ExecContext(ctx, query,
		milestone.ChannelID,
		milestone.MemberName,
		string(milestone.Type),
		milestone.Value,
		milestone.AchievedAt,
		milestone.Notified,
	)

	if err != nil {
		return fmt.Errorf("failed to save milestone: %w", err)
	}

	r.logger.Info("Milestone saved",
		zap.String("member", milestone.MemberName),
		zap.String("type", string(milestone.Type)),
		zap.Uint64("value", milestone.Value),
	)

	return nil
}

func (r *YouTubeStatsRepository) GetUnnotifiedChanges(ctx context.Context, limit int) ([]*domain.StatsChange, error) {
	query := `
		SELECT channel_id, member_name, subscriber_change, video_change, view_change, detected_at
		FROM youtube_stats_changes
		WHERE notified = false
		ORDER BY detected_at DESC
		LIMIT $1
	`

	rows, err := r.db.QueryContext(ctx, query, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query unnotified changes: %w", err)
	}
	defer rows.Close()

	var changes []*domain.StatsChange
	for rows.Next() {
		var change domain.StatsChange
		if err := rows.Scan(
			&change.ChannelID,
			&change.MemberName,
			&change.SubscriberChange,
			&change.VideoChange,
			&change.ViewChange,
			&change.DetectedAt,
		); err != nil {
			r.logger.Warn("Failed to scan change row", zap.Error(err))
			continue
		}
		changes = append(changes, &change)
	}

	return changes, nil
}

func (r *YouTubeStatsRepository) MarkChangeNotified(ctx context.Context, channelID string, detectedAt time.Time) error {
	query := `
		UPDATE youtube_stats_changes
		SET notified = true
		WHERE channel_id = $1 AND detected_at = $2
	`

	_, err := r.db.ExecContext(ctx, query, channelID, detectedAt)
	if err != nil {
		return fmt.Errorf("failed to mark change notified: %w", err)
	}

	return nil
}

func (r *YouTubeStatsRepository) GetTopGainers(ctx context.Context, since time.Time, limit int) ([]domain.RankEntry, error) {
	query := `
		WITH latest AS (
			SELECT DISTINCT ON (channel_id)
				channel_id, member_name, subscribers
			FROM youtube_stats_history
			WHERE time >= $1
			ORDER BY channel_id, time DESC
		),
		earliest AS (
			SELECT DISTINCT ON (channel_id)
				channel_id, subscribers
			FROM youtube_stats_history
			WHERE time >= $1
			ORDER BY channel_id, time ASC
		)
		SELECT
			latest.channel_id,
			latest.member_name,
			(latest.subscribers - earliest.subscribers) AS gain
		FROM latest
		JOIN earliest ON latest.channel_id = earliest.channel_id
		WHERE (latest.subscribers - earliest.subscribers) > 0
		ORDER BY gain DESC
		LIMIT $2
	`

	rows, err := r.db.QueryContext(ctx, query, since, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query top gainers: %w", err)
	}
	defer rows.Close()

	var entries []domain.RankEntry
	rank := 1
	for rows.Next() {
		var entry domain.RankEntry
		if err := rows.Scan(&entry.ChannelID, &entry.MemberName, &entry.Value); err != nil {
			r.logger.Warn("Failed to scan rank entry", zap.Error(err))
			continue
		}
		entry.Rank = rank
		entries = append(entries, entry)
		rank++
	}

	return entries, nil
}
