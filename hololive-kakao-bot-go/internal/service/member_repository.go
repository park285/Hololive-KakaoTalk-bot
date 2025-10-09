package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
)

type MemberRepository struct {
	db     *sql.DB
	logger *zap.Logger
}

func NewMemberRepository(postgres *PostgresService, logger *zap.Logger) *MemberRepository {
	return &MemberRepository{
		db:     postgres.GetDB(),
		logger: logger,
	}
}

// FindByChannelID retrieves member by YouTube channel ID
func (r *MemberRepository) FindByChannelID(ctx context.Context, channelID string) (*domain.Member, error) {
	query := `
		SELECT id, slug, channel_id, english_name, japanese_name, korean_name,
		       status, is_graduated, aliases
		FROM members
		WHERE channel_id = $1
		LIMIT 1
	`

	var (
		id           int
		slug         string
		channelIDVal sql.NullString
		englishName  string
		japaneseName sql.NullString
		koreanName   sql.NullString
		status       string
		isGraduated  bool
		aliasesJSON  []byte
	)

	err := r.db.QueryRowContext(ctx, query, channelID).Scan(
		&id, &slug, &channelIDVal, &englishName, &japaneseName, &koreanName,
		&status, &isGraduated, &aliasesJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query member by channel_id: %w", err)
	}

	return r.scanMember(id, slug, channelIDVal, englishName, japaneseName, koreanName, status, isGraduated, aliasesJSON)
}

// FindByName retrieves member by English name
func (r *MemberRepository) FindByName(ctx context.Context, name string) (*domain.Member, error) {
	query := `
		SELECT id, slug, channel_id, english_name, japanese_name, korean_name,
		       status, is_graduated, aliases
		FROM members
		WHERE english_name = $1
		LIMIT 1
	`

	var (
		id           int
		slug         string
		channelID    sql.NullString
		englishName  string
		japaneseName sql.NullString
		koreanName   sql.NullString
		status       string
		isGraduated  bool
		aliasesJSON  []byte
	)

	err := r.db.QueryRowContext(ctx, query, name).Scan(
		&id, &slug, &channelID, &englishName, &japaneseName, &koreanName,
		&status, &isGraduated, &aliasesJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query member by name: %w", err)
	}

	return r.scanMember(id, slug, channelID, englishName, japaneseName, koreanName, status, isGraduated, aliasesJSON)
}

// FindByAlias searches member by any alias (Korean or Japanese)
func (r *MemberRepository) FindByAlias(ctx context.Context, alias string) (*domain.Member, error) {
	query := `
		SELECT m.id, m.slug, m.channel_id, m.english_name, m.japanese_name, m.korean_name,
		       m.status, m.is_graduated, m.aliases
		FROM members m
		WHERE m.aliases->'ko' ? $1
		   OR m.aliases->'ja' ? $1
		   OR m.english_name ILIKE $1
		   OR m.japanese_name ILIKE $1
		   OR m.korean_name ILIKE $1
		LIMIT 1
	`

	var (
		id           int
		slug         string
		channelID    sql.NullString
		englishName  string
		japaneseName sql.NullString
		koreanName   sql.NullString
		status       string
		isGraduated  bool
		aliasesJSON  []byte
	)

	err := r.db.QueryRowContext(ctx, query, alias).Scan(
		&id, &slug, &channelID, &englishName, &japaneseName, &koreanName,
		&status, &isGraduated, &aliasesJSON,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query member by alias: %w", err)
	}

	return r.scanMember(id, slug, channelID, englishName, japaneseName, koreanName, status, isGraduated, aliasesJSON)
}

// GetAllChannelIDs returns all channel IDs
func (r *MemberRepository) GetAllChannelIDs(ctx context.Context) ([]string, error) {
	query := `
		SELECT channel_id
		FROM members
		WHERE channel_id IS NOT NULL
		ORDER BY english_name
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query channel ids: %w", err)
	}
	defer rows.Close()

	var channelIDs []string
	for rows.Next() {
		var channelID string
		if err := rows.Scan(&channelID); err != nil {
			r.logger.Warn("Failed to scan channel ID", zap.Error(err))
			continue
		}
		channelIDs = append(channelIDs, channelID)
	}

	return channelIDs, nil
}

// GetAllMembers returns all members (for initial cache warming)
func (r *MemberRepository) GetAllMembers(ctx context.Context) ([]*domain.Member, error) {
	query := `
		SELECT id, slug, channel_id, english_name, japanese_name, korean_name,
		       status, is_graduated, aliases
		FROM members
		ORDER BY english_name
	`

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query all members: %w", err)
	}
	defer rows.Close()

	var members []*domain.Member
	for rows.Next() {
		var (
			id           int
			slug         string
			channelID    sql.NullString
			englishName  string
			japaneseName sql.NullString
			koreanName   sql.NullString
			status       string
			isGraduated  bool
			aliasesJSON  []byte
		)

		if err := rows.Scan(&id, &slug, &channelID, &englishName, &japaneseName, &koreanName,
			&status, &isGraduated, &aliasesJSON); err != nil {
			r.logger.Warn("Failed to scan member row", zap.Error(err))
			continue
		}

		member, err := r.scanMember(id, slug, channelID, englishName, japaneseName, koreanName, status, isGraduated, aliasesJSON)
		if err != nil {
			r.logger.Warn("Failed to parse member", zap.String("name", englishName), zap.Error(err))
			continue
		}

		members = append(members, member)
	}

	return members, nil
}

// scanMember converts DB row to domain.Member
func (r *MemberRepository) scanMember(
	id int,
	slug string,
	channelID sql.NullString,
	englishName string,
	japaneseName sql.NullString,
	koreanName sql.NullString,
	status string,
	isGraduated bool,
	aliasesJSON []byte,
) (*domain.Member, error) {
	var aliases domain.Aliases
	if err := json.Unmarshal(aliasesJSON, &aliases); err != nil {
		return nil, fmt.Errorf("failed to unmarshal aliases: %w", err)
	}

	member := &domain.Member{
		Name:        englishName,
		Aliases:     &aliases,
		IsGraduated: isGraduated,
	}

	if channelID.Valid {
		member.ChannelID = channelID.String
	}
	if japaneseName.Valid {
		member.NameJa = japaneseName.String
	}
	if koreanName.Valid {
		member.NameKo = koreanName.String
	}

	return member, nil
}
