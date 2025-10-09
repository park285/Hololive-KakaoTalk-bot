-- Members and Profiles Schema Migration
-- Purpose: Migrate JSON data to PostgreSQL with JSONB for flexible fields
-- Author: Migration from JSON files to DB
-- Date: 2025-10-09

-- ============================================================================
-- 1. MEMBERS TABLE (Core member information)
-- ============================================================================
CREATE TABLE IF NOT EXISTS members (
    id SERIAL PRIMARY KEY,

    -- Identifiers
    slug VARCHAR(100) UNIQUE NOT NULL,               -- URL slug: irys, gawr-gura, fuwawa-abyssgard
    channel_id VARCHAR(64) UNIQUE,                   -- YouTube Channel ID

    -- Names (Multiple languages)
    english_name VARCHAR(100) NOT NULL,              -- Official English name
    japanese_name VARCHAR(100),                      -- Official Japanese name
    korean_name VARCHAR(100),                        -- Korean translation

    -- Status
    status VARCHAR(20) NOT NULL DEFAULT 'active',    -- active, graduated, ended
    is_graduated BOOLEAN DEFAULT false,              -- Backward compatibility

    -- Official data
    official_link TEXT,                              -- hololive.hololivepro.com link

    -- Flexible metadata (JSONB)
    aliases JSONB DEFAULT '{"ko": [], "ja": []}'::jsonb,  -- {"ko": ["별명1"], "ja": ["エイリアス"]}

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- Constraints
    CONSTRAINT check_status CHECK (status IN ('active', 'graduated', 'ended')),
    CONSTRAINT check_aliases_structure CHECK (
        jsonb_typeof(aliases) = 'object' AND
        aliases ? 'ko' AND
        aliases ? 'ja' AND
        jsonb_typeof(aliases->'ko') = 'array' AND
        jsonb_typeof(aliases->'ja') = 'array'
    )
);

-- Indexes
CREATE INDEX idx_members_channel_id ON members(channel_id) WHERE channel_id IS NOT NULL;
CREATE INDEX idx_members_slug ON members(slug);
CREATE INDEX idx_members_status ON members(status);
CREATE INDEX idx_members_english_name ON members(english_name);
CREATE INDEX idx_members_aliases_gin ON members USING GIN(aliases jsonb_path_ops);

-- Full-text search index for names
CREATE INDEX idx_members_name_search ON members USING GIN(
    to_tsvector('simple',
        COALESCE(english_name, '') || ' ' ||
        COALESCE(japanese_name, '') || ' ' ||
        COALESCE(korean_name, '')
    )
);

-- ============================================================================
-- 2. MEMBER PROFILES TABLE (Localized profile content)
-- ============================================================================
CREATE TABLE IF NOT EXISTS member_profiles (
    id SERIAL PRIMARY KEY,
    member_id INTEGER NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    language VARCHAR(10) NOT NULL,                   -- ja, ko, en

    -- Profile content
    display_name TEXT,                               -- Display name with native script
    catchphrase TEXT,                                -- Intro/catchphrase
    description TEXT,                                -- Long-form description
    summary TEXT,                                    -- Short summary

    -- Highlights (JSONB array of strings)
    highlights JSONB DEFAULT '[]'::jsonb,            -- ["특징1", "특징2"]

    -- Structured profile data (JSONB array of label-value pairs)
    profile_data JSONB DEFAULT '[]'::jsonb,          -- [{"label": "생일", "value": "7월 1일"}, ...]

    -- Timestamps
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),

    -- Constraints
    UNIQUE(member_id, language),
    CONSTRAINT check_language CHECK (language IN ('ja', 'ko', 'en')),
    CONSTRAINT check_highlights_array CHECK (jsonb_typeof(highlights) = 'array'),
    CONSTRAINT check_profile_data_array CHECK (jsonb_typeof(profile_data) = 'array')
);

-- Indexes
CREATE INDEX idx_profiles_member ON member_profiles(member_id);
CREATE INDEX idx_profiles_language ON member_profiles(language);
CREATE INDEX idx_profiles_member_lang ON member_profiles(member_id, language);

-- ============================================================================
-- 3. AUTO-UPDATE TRIGGER FOR updated_at
-- ============================================================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER members_updated_at_trigger
    BEFORE UPDATE ON members
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER profiles_updated_at_trigger
    BEFORE UPDATE ON member_profiles
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- 4. HELPER FUNCTIONS
-- ============================================================================

-- Find member by any alias
CREATE OR REPLACE FUNCTION find_member_by_alias(search_alias TEXT)
RETURNS TABLE (
    id INTEGER,
    english_name VARCHAR(100),
    channel_id VARCHAR(64)
) AS $$
BEGIN
    RETURN QUERY
    SELECT m.id, m.english_name, m.channel_id
    FROM members m
    WHERE
        m.aliases->'ko' ? search_alias OR
        m.aliases->'ja' ? search_alias OR
        m.english_name ILIKE search_alias OR
        m.japanese_name ILIKE search_alias OR
        m.korean_name ILIKE search_alias;
END;
$$ LANGUAGE plpgsql STABLE;

-- Get all aliases for a member
CREATE OR REPLACE FUNCTION get_all_aliases(member_id_param INTEGER)
RETURNS TEXT[] AS $$
DECLARE
    ko_aliases JSONB;
    ja_aliases JSONB;
    result TEXT[];
BEGIN
    SELECT aliases->'ko', aliases->'ja'
    INTO ko_aliases, ja_aliases
    FROM members
    WHERE id = member_id_param;

    -- Convert JSONB arrays to TEXT[]
    SELECT ARRAY(
        SELECT jsonb_array_elements_text(ko_aliases)
        UNION ALL
        SELECT jsonb_array_elements_text(ja_aliases)
    ) INTO result;

    RETURN result;
END;
$$ LANGUAGE plpgsql STABLE;

-- ============================================================================
-- 5. DATA VALIDATION VIEWS
-- ============================================================================

-- Members without channel IDs (might be staff or special cases)
CREATE VIEW members_without_channels AS
SELECT id, english_name, status
FROM members
WHERE channel_id IS NULL;

-- Members without profiles
CREATE VIEW members_without_profiles AS
SELECT m.id, m.english_name, m.status
FROM members m
LEFT JOIN member_profiles mp ON m.id = mp.member_id
WHERE mp.id IS NULL;

-- Profile completeness check
CREATE VIEW profile_completeness AS
SELECT
    m.english_name,
    m.status,
    COUNT(DISTINCT mp.language) as language_count,
    ARRAY_AGG(DISTINCT mp.language ORDER BY mp.language) as available_languages,
    CASE WHEN COUNT(DISTINCT mp.language) >= 2 THEN 'complete' ELSE 'incomplete' END as completeness_status
FROM members m
LEFT JOIN member_profiles mp ON m.id = mp.member_id
GROUP BY m.id, m.english_name, m.status
ORDER BY language_count DESC, m.english_name;

-- ============================================================================
-- 6. COMMENTS FOR DOCUMENTATION
-- ============================================================================
COMMENT ON TABLE members IS 'Core member information with flexible JSONB fields for aliases';
COMMENT ON TABLE member_profiles IS 'Localized profile content (ja, ko, en) with JSONB for structured data';
COMMENT ON COLUMN members.aliases IS 'JSONB: {"ko": ["별명1"], "ja": ["エイリアス"]}';
COMMENT ON COLUMN member_profiles.highlights IS 'JSONB array: ["특징1", "특징2", "특징3"]';
COMMENT ON COLUMN member_profiles.profile_data IS 'JSONB array: [{"label": "생일", "value": "7월 1일"}]';
COMMENT ON FUNCTION find_member_by_alias(TEXT) IS 'Search member by any alias in Korean or Japanese';
COMMENT ON FUNCTION get_all_aliases(INTEGER) IS 'Get all aliases (ko + ja) for a member as TEXT array';
