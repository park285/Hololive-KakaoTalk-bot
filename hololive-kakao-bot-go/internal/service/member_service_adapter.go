package service

import (
	"context"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

// MemberServiceAdapter adapts MemberCache to legacy MembersData interface
// This provides backward compatibility while migrating to new architecture
type MemberServiceAdapter struct {
	cache *MemberCache
	ctx   context.Context
}

func NewMemberServiceAdapter(cache *MemberCache) *MemberServiceAdapter {
	return &MemberServiceAdapter{
		cache: cache,
		ctx:   context.Background(),
	}
}

// FindMemberByChannelID implements MembersData interface
func (a *MemberServiceAdapter) FindMemberByChannelID(channelID string) *domain.Member {
	member, err := a.cache.GetByChannelID(a.ctx, channelID)
	if err != nil {
		return nil
	}
	return member
}

// FindMemberByName implements MembersData interface
func (a *MemberServiceAdapter) FindMemberByName(name string) *domain.Member {
	member, err := a.cache.GetByName(a.ctx, name)
	if err != nil {
		return nil
	}
	return member
}

// FindMemberByAlias implements MembersData interface
func (a *MemberServiceAdapter) FindMemberByAlias(alias string) *domain.Member {
	member, err := a.cache.FindByAlias(a.ctx, alias)
	if err != nil {
		return nil
	}
	return member
}

// GetChannelIDs implements MemberDataProvider interface
func (a *MemberServiceAdapter) GetChannelIDs() []string {
	channelIDs, err := a.cache.GetAllChannelIDs(a.ctx)
	if err != nil {
		return []string{}
	}
	return channelIDs
}

// GetAllMembers implements MemberDataProvider interface
func (a *MemberServiceAdapter) GetAllMembers() []*domain.Member {
	members, err := a.cache.repo.GetAllMembers(a.ctx)
	if err != nil {
		return []*domain.Member{}
	}
	return members
}

// WithContext creates a new adapter with custom context
func (a *MemberServiceAdapter) WithContext(ctx context.Context) domain.MemberDataProvider {
	if ctx == nil {
		ctx = context.Background()
	}
	return &MemberServiceAdapter{
		cache: a.cache,
		ctx:   ctx,
	}
}
