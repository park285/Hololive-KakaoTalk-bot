package domain

import "context"

// MemberDataProvider defines interface for member data access
// This abstraction allows switching between JSON (legacy) and PostgreSQL (new)
type MemberDataProvider interface {
	FindMemberByChannelID(channelID string) *Member
	FindMemberByName(name string) *Member
	FindMemberByAlias(alias string) *Member
	GetChannelIDs() []string
	GetAllMembers() []*Member // For iteration (legacy compatibility)
	WithContext(ctx context.Context) MemberDataProvider
}

// Ensure MembersData implements the interface (legacy)
var _ MemberDataProvider = (*MembersData)(nil)
