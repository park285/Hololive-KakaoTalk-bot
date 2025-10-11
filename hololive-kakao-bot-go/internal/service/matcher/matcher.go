package matcher

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/cache"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/holodex"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"go.uber.org/zap"
)

type MatchCacheEntry struct {
	Channel   *domain.Channel
	Timestamp time.Time
}

type matchCandidate struct {
	channelID  string
	memberName string
	source     string
}

type ChannelSelector interface {
	SelectBestChannel(ctx context.Context, query string, candidates []*domain.Channel) (*domain.Channel, error)
}

type MemberMatcher struct {
	membersData           domain.MemberDataProvider
	aliasToName           map[string]string
	cache                 *cache.CacheService
	holodex               *holodex.HolodexService
	selector              ChannelSelector
	logger                *zap.Logger
	matchCache            map[string]*MatchCacheEntry
	matchCacheMu          sync.RWMutex
	matchCacheTTL         time.Duration
	matchCacheLastCleanup time.Time
}

func NewMemberMatcher(
	membersData domain.MemberDataProvider,
	cache *cache.CacheService,
	holodex *holodex.HolodexService,
	selector ChannelSelector,
	logger *zap.Logger,
	ctx context.Context,
) *MemberMatcher {
	if ctx == nil {
		ctx = context.Background()
	}

	mm := &MemberMatcher{
		membersData:           membersData,
		cache:                 cache,
		holodex:               holodex,
		selector:              selector,
		logger:                logger,
		matchCache:            make(map[string]*MatchCacheEntry),
		matchCacheTTL:         1 * time.Minute,
		matchCacheLastCleanup: time.Now(),
	}

	provider := membersData.WithContext(ctx)
	mm.aliasToName = mm.buildAliasMap(provider)

	logger.Info("MemberMatcher initialized",
		zap.Int("members", len(provider.GetAllMembers())),
		zap.Int("aliases", len(mm.aliasToName)),
	)

	return mm
}

func (mm *MemberMatcher) buildAliasMap(provider domain.MemberDataProvider) map[string]string {
	aliasMap := make(map[string]string)

	for _, member := range provider.GetAllMembers() {
		aliasMap[strings.ToLower(member.Name)] = member.Name

		if member.NameJa != "" {
			aliasMap[strings.ToLower(member.NameJa)] = member.Name
		}

		if member.Aliases != nil && len(member.Aliases.Ko) > 0 {
			for _, alias := range member.Aliases.Ko {
				aliasMap[strings.ToLower(alias)] = member.Name
			}
		}

		if member.Aliases != nil && len(member.Aliases.Ja) > 0 {
			for _, alias := range member.Aliases.Ja {
				aliasMap[strings.ToLower(alias)] = member.Name
			}
		}
	}

	return aliasMap
}

// tryExactAliasMatch attempts exact match via pre-built alias map without hitting Holodex repeatedly.
func (mm *MemberMatcher) tryExactAliasMatch(ctx context.Context, provider domain.MemberDataProvider, queryNorm string) *matchCandidate {
	englishName, found := mm.aliasToName[queryNorm]
	if !found {
		return nil
	}

	if member := provider.FindMemberByName(englishName); member != nil && member.ChannelID != "" {
		return mm.candidateFromMember(member, "alias-map")
	}

	channelID, err := mm.cache.GetMemberChannelID(ctx, englishName)
	if err != nil {
		mm.logger.Warn("Failed to resolve alias from cache",
			zap.String("member", englishName),
			zap.Error(err),
		)
		return nil
	}
	if channelID == "" {
		return nil
	}

	return mm.candidateFromDynamic(provider, englishName, channelID, "alias-redis")
}

// tryExactRedisMatch attempts exact match in dynamic Redis data without immediate Holodex calls.
func (mm *MemberMatcher) tryExactRedisMatch(provider domain.MemberDataProvider, query string, dynamicMembers map[string]string) *matchCandidate {
	for name, channelID := range dynamicMembers {
		if strings.EqualFold(name, query) {
			return mm.candidateFromDynamic(provider, name, channelID, "redis-exact")
		}
	}
	return nil
}

// tryPartialStaticMatch attempts partial match in static member data.
func (mm *MemberMatcher) tryPartialStaticMatch(provider domain.MemberDataProvider, queryNorm string) *matchCandidate {
	for _, member := range provider.GetAllMembers() {
		nameNorm := util.Normalize(member.Name)
		if strings.Contains(nameNorm, queryNorm) || strings.Contains(queryNorm, nameNorm) {
			return mm.candidateFromMember(member, "static-partial")
		}
	}
	return nil
}

// tryPartialRedisMatch attempts partial match in dynamic Redis data.
func (mm *MemberMatcher) tryPartialRedisMatch(provider domain.MemberDataProvider, queryNorm string, dynamicMembers map[string]string) *matchCandidate {
	for name, channelID := range dynamicMembers {
		nameNorm := util.Normalize(name)
		if strings.Contains(nameNorm, queryNorm) || strings.Contains(queryNorm, nameNorm) {
			return mm.candidateFromDynamic(provider, name, channelID, "redis-partial")
		}
	}
	return nil
}

// tryPartialAliasMatch attempts partial match across all aliases.
func (mm *MemberMatcher) tryPartialAliasMatch(provider domain.MemberDataProvider, queryNorm string) *matchCandidate {
	for _, member := range provider.GetAllMembers() {
		for _, alias := range member.GetAllAliases() {
			aliasNorm := util.Normalize(alias)
			if strings.Contains(aliasNorm, queryNorm) || strings.Contains(queryNorm, aliasNorm) {
				return mm.candidateFromMember(member, "alias-partial")
			}
		}
	}
	return nil
}

func (mm *MemberMatcher) candidateFromMember(member *domain.Member, source string) *matchCandidate {
	if member == nil || member.ChannelID == "" {
		return nil
	}

	name := member.Name
	if name == "" {
		name = member.NameJa
	}
	if name == "" {
		name = member.ChannelID
	}

	return &matchCandidate{
		channelID:  member.ChannelID,
		memberName: name,
		source:     source,
	}
}

func (mm *MemberMatcher) candidateFromDynamic(provider domain.MemberDataProvider, name, channelID, source string) *matchCandidate {
	if channelID == "" {
		return nil
	}

	if provider != nil {
		if member := provider.FindMemberByChannelID(channelID); member != nil {
			if candidate := mm.candidateFromMember(member, source); candidate != nil {
				return candidate
			}
		}
	}

	displayName := name
	if displayName == "" {
		displayName = channelID
	}

	return &matchCandidate{
		channelID:  channelID,
		memberName: displayName,
		source:     source,
	}
}

// tryHolodexAPISearch searches via external Holodex API
func (mm *MemberMatcher) tryHolodexAPISearch(ctx context.Context, query string) ([]*domain.Channel, error) {
	mm.logger.Info("Fallback: Holodex API search", zap.String("query", query))

	channels, err := mm.holodex.SearchChannels(ctx, query)
	if err != nil {
		mm.logger.Warn("Holodex API failed", zap.Error(err))
		return nil, nil
	}

	if len(channels) == 0 {
		mm.logger.Warn("No Holodex results")
		return nil, nil
	}

	return channels, nil
}

func (mm *MemberMatcher) hydrateChannel(ctx context.Context, candidate *matchCandidate) (*domain.Channel, error) {
	if candidate == nil {
		return nil, nil
	}

	fallback := &domain.Channel{
		ID:   candidate.channelID,
		Name: candidate.memberName,
	}
	if candidate.memberName != "" {
		fallback.EnglishName = toStringPtr(candidate.memberName)
	}

	if mm.holodex == nil {
		return fallback, nil
	}

	channel, err := mm.holodex.GetChannel(ctx, candidate.channelID)
	if err != nil {
		mm.logger.Warn("Failed to fetch channel from Holodex",
			zap.String("channel_id", candidate.channelID),
			zap.String("source", candidate.source),
			zap.Error(err),
		)
		return fallback, nil
	}

	if channel == nil {
		mm.logger.Warn("Holodex returned empty channel",
			zap.String("channel_id", candidate.channelID),
			zap.String("source", candidate.source),
		)
		return fallback, nil
	}

	if candidate.memberName != "" {
		if channel.Name == "" {
			channel.Name = candidate.memberName
		}
		if channel.EnglishName == nil {
			channel.EnglishName = toStringPtr(candidate.memberName)
		}
	}

	return channel, nil
}

func (mm *MemberMatcher) finalizeCandidate(ctx context.Context, candidate *matchCandidate) (*domain.Channel, error) {
	if candidate == nil {
		return nil, nil
	}

	if candidate.channelID == "" {
		mm.logger.Warn("Match candidate missing channel ID",
			zap.String("member", candidate.memberName),
			zap.String("source", candidate.source),
		)
		return nil, nil
	}

	channel, err := mm.hydrateChannel(ctx, candidate)
	if err != nil {
		return nil, err
	}

	if channel != nil {
		mm.logger.Debug("Match candidate resolved",
			zap.String("channel_id", candidate.channelID),
			zap.String("member", candidate.memberName),
			zap.String("source", candidate.source),
		)
	}

	return channel, nil
}

func (mm *MemberMatcher) maybeCleanupMatchCache() {
	mm.matchCacheMu.Lock()
	defer mm.matchCacheMu.Unlock()

	if time.Since(mm.matchCacheLastCleanup) < mm.matchCacheTTL {
		return
	}

	cutoff := time.Now().Add(-mm.matchCacheTTL)
	for key, entry := range mm.matchCache {
		if entry == nil || entry.Timestamp.Before(cutoff) {
			delete(mm.matchCache, key)
		}
	}

	mm.matchCacheLastCleanup = time.Now()
}

func toStringPtr(value string) *string {
	if value == "" {
		return nil
	}
	copy := value
	return &copy
}

// selectBestFromCandidates selects best channel from multiple candidates
func (mm *MemberMatcher) selectBestFromCandidates(ctx context.Context, query string, channels []*domain.Channel) (*domain.Channel, error) {
	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		mm.logger.Info("Single Holodex result", zap.String("channel", channels[0].Name))
		return channels[0], nil
	}

	// Multiple candidates - use selector strategy
	if mm.selector == nil {
		mm.logger.Info("No Gemini service - using first result", zap.String("channel", channels[0].Name))
		return channels[0], nil
	}

	mm.logger.Info("Channel selector evaluation", zap.Int("candidates", len(channels)))

	selected, err := mm.selector.SelectBestChannel(ctx, query, channels)
	if err != nil {
		mm.logger.Warn("Channel selector failed", zap.Error(err))
		return nil, nil
	}

	if selected != nil {
		mm.logger.Info("Selector chose channel", zap.String("channel", selected.Name))
		return selected, nil
	}

	mm.logger.Warn("Selector returned no match")
	return nil, nil
}

// loadDynamicMembers fetches member data from Redis cache
func (mm *MemberMatcher) loadDynamicMembers(ctx context.Context) map[string]string {
	members, err := mm.cache.GetAllMembers(ctx)
	if err != nil {
		mm.logger.Warn("Failed to load dynamic members", zap.Error(err))
		return map[string]string{}
	}
	return members
}

func (mm *MemberMatcher) FindBestMatch(ctx context.Context, query string) (*domain.Channel, error) {
	normalizedQuery := util.Normalize(query)
	cacheKey := fmt.Sprintf("match:%s", normalizedQuery)

	mm.matchCacheMu.RLock()
	cached, found := mm.matchCache[cacheKey]
	mm.matchCacheMu.RUnlock()

	if found {
		age := time.Since(cached.Timestamp)
		if age < mm.matchCacheTTL {
			return cached.Channel, nil
		}

		mm.matchCacheMu.Lock()
		delete(mm.matchCache, cacheKey)
		mm.matchCacheMu.Unlock()
	}

	channel, err := mm.findBestMatchImpl(ctx, query)

	mm.matchCacheMu.Lock()
	mm.matchCache[cacheKey] = &MatchCacheEntry{
		Channel:   channel,
		Timestamp: time.Now(),
	}
	mm.matchCacheMu.Unlock()

	mm.maybeCleanupMatchCache()

	return channel, err
}

func (mm *MemberMatcher) findBestMatchImpl(ctx context.Context, query string) (*domain.Channel, error) {
	provider := mm.membersData.WithContext(ctx)
	queryNorm := util.NormalizeSuffix(query)

	// Strategy 1: Exact alias match (fastest)
	if channel, err := mm.finalizeCandidate(ctx, mm.tryExactAliasMatch(ctx, provider, queryNorm)); err != nil || channel != nil {
		return channel, err
	}

	// Load dynamic members once for strategies 2 & 4
	dynamicMembers := mm.loadDynamicMembers(ctx)

	// Strategy 2: Exact match in Redis
	if channel, err := mm.finalizeCandidate(ctx, mm.tryExactRedisMatch(provider, query, dynamicMembers)); err != nil || channel != nil {
		return channel, err
	}

	// Strategy 3: Partial match in static data
	if channel, err := mm.finalizeCandidate(ctx, mm.tryPartialStaticMatch(provider, queryNorm)); err != nil || channel != nil {
		return channel, err
	}

	// Strategy 4: Partial match in Redis
	if channel, err := mm.finalizeCandidate(ctx, mm.tryPartialRedisMatch(provider, queryNorm, dynamicMembers)); err != nil || channel != nil {
		return channel, err
	}

	// Strategy 5: Partial alias match
	if channel, err := mm.finalizeCandidate(ctx, mm.tryPartialAliasMatch(provider, queryNorm)); err != nil || channel != nil {
		return channel, err
	}

	// Strategy 6: Holodex API search
	channels, err := mm.tryHolodexAPISearch(ctx, query)
	if err != nil {
		return nil, err
	}

	// Strategy 7: Select best from multiple candidates (with Gemini AI if available)
	return mm.selectBestFromCandidates(ctx, query, channels)
}

func (mm *MemberMatcher) GetAllMembers() []*domain.Member {
	return mm.membersData.WithContext(context.Background()).GetAllMembers()
}

func (mm *MemberMatcher) GetMemberByChannelID(channelID string) *domain.Member {
	return mm.membersData.WithContext(context.Background()).FindMemberByChannelID(channelID)
}
