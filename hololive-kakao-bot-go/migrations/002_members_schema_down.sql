-- Rollback script for 002_members_schema.sql
-- Purpose: Clean rollback of members and profiles tables
-- WARNING: This will DELETE ALL DATA in members and member_profiles tables

-- ============================================================================
-- DROP VIEWS
-- ============================================================================
DROP VIEW IF EXISTS profile_completeness;
DROP VIEW IF EXISTS members_without_profiles;
DROP VIEW IF EXISTS members_without_channels;

-- ============================================================================
-- DROP FUNCTIONS
-- ============================================================================
DROP FUNCTION IF EXISTS get_all_aliases(INTEGER);
DROP FUNCTION IF EXISTS find_member_by_alias(TEXT);
DROP FUNCTION IF EXISTS update_updated_at_column() CASCADE;

-- ============================================================================
-- DROP TABLES (CASCADE to remove dependent objects)
-- ============================================================================
DROP TABLE IF EXISTS member_profiles CASCADE;
DROP TABLE IF EXISTS members CASCADE;

-- Note: Triggers are automatically dropped with functions/tables
