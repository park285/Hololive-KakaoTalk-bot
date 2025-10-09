-- YouTube Statistics Schema for TimescaleDB
-- Purpose: Track channel statistics over time for trending analysis and milestones

-- 1. Time-series statistics table
CREATE TABLE IF NOT EXISTS youtube_stats_history (
    time        TIMESTAMPTZ NOT NULL,
    channel_id  VARCHAR(64) NOT NULL,
    member_name VARCHAR(100),
    subscribers BIGINT NOT NULL,
    videos      BIGINT NOT NULL,
    views       BIGINT NOT NULL,
    PRIMARY KEY (time, channel_id)
);

-- Convert to hypertable (TimescaleDB time-series optimization)
SELECT create_hypertable(
    'youtube_stats_history',
    'time',
    if_not_exists => TRUE,
    chunk_time_interval => INTERVAL '7 days'
);

-- Index for channel lookups
CREATE INDEX IF NOT EXISTS idx_youtube_stats_channel
    ON youtube_stats_history (channel_id, time DESC);

-- Index for member name lookups
CREATE INDEX IF NOT EXISTS idx_youtube_stats_member
    ON youtube_stats_history (member_name, time DESC);

-- 2. Milestones table
CREATE TABLE IF NOT EXISTS youtube_milestones (
    id          SERIAL PRIMARY KEY,
    channel_id  VARCHAR(64) NOT NULL,
    member_name VARCHAR(100),
    type        VARCHAR(20) NOT NULL,  -- 'subscribers', 'videos', 'views'
    value       BIGINT NOT NULL,       -- e.g., 1000000 for 1M subscribers
    achieved_at TIMESTAMPTZ NOT NULL,
    notified    BOOLEAN DEFAULT false,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

-- Index for querying unnotified milestones
CREATE INDEX IF NOT EXISTS idx_milestones_unnotified
    ON youtube_milestones (notified, achieved_at DESC)
    WHERE notified = false;

-- Index for channel milestones
CREATE INDEX IF NOT EXISTS idx_milestones_channel
    ON youtube_milestones (channel_id, achieved_at DESC);

-- 3. Daily summary table
CREATE TABLE IF NOT EXISTS youtube_daily_summary (
    date              DATE PRIMARY KEY,
    total_changes     INT DEFAULT 0,
    milestones_count  INT DEFAULT 0,
    new_videos_count  INT DEFAULT 0,
    top_gainers       JSONB,  -- Array of {member_name, subscriber_change}
    top_uploaders     JSONB,  -- Array of {member_name, video_count}
    summary_data      JSONB,
    created_at        TIMESTAMPTZ DEFAULT NOW()
);

-- 4. Stats changes tracking table (for notifications)
CREATE TABLE IF NOT EXISTS youtube_stats_changes (
    id                 SERIAL PRIMARY KEY,
    channel_id         VARCHAR(64) NOT NULL,
    member_name        VARCHAR(100),
    subscriber_change  BIGINT DEFAULT 0,
    video_change       BIGINT DEFAULT 0,
    view_change        BIGINT DEFAULT 0,
    previous_subs      BIGINT,
    current_subs       BIGINT,
    previous_videos    BIGINT,
    current_videos     BIGINT,
    detected_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    notified           BOOLEAN DEFAULT false
);

-- Index for querying recent changes
CREATE INDEX IF NOT EXISTS idx_changes_detected
    ON youtube_stats_changes (detected_at DESC);

-- Index for unnotified changes
CREATE INDEX IF NOT EXISTS idx_changes_unnotified
    ON youtube_stats_changes (notified, detected_at DESC)
    WHERE notified = false;

-- 5. Retention policy (keep 90 days of raw data, compress older)
SELECT add_retention_policy(
    'youtube_stats_history',
    INTERVAL '90 days',
    if_not_exists => TRUE
);

-- 6. Compression policy (compress data older than 7 days)
SELECT add_compression_policy(
    'youtube_stats_history',
    INTERVAL '7 days',
    if_not_exists => TRUE
);

-- 7. Continuous aggregate for daily statistics (materialized view)
CREATE MATERIALIZED VIEW IF NOT EXISTS youtube_daily_stats
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', time) AS day,
    channel_id,
    member_name,
    AVG(subscribers) AS avg_subscribers,
    MAX(subscribers) AS max_subscribers,
    MIN(subscribers) AS min_subscribers,
    AVG(videos) AS avg_videos,
    MAX(videos) - MIN(videos) AS videos_uploaded
FROM youtube_stats_history
GROUP BY day, channel_id, member_name
WITH NO DATA;

-- Refresh policy for continuous aggregate
SELECT add_continuous_aggregate_policy(
    'youtube_daily_stats',
    start_offset => INTERVAL '3 days',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

-- Comments
COMMENT ON TABLE youtube_stats_history IS 'Time-series YouTube channel statistics';
COMMENT ON TABLE youtube_milestones IS 'Significant milestones achieved by channels';
COMMENT ON TABLE youtube_daily_summary IS 'Daily aggregated summary of all channels';
COMMENT ON TABLE youtube_stats_changes IS 'Detected changes for notification purposes';
COMMENT ON MATERIALIZED VIEW youtube_daily_stats IS 'Materialized daily statistics per channel';
