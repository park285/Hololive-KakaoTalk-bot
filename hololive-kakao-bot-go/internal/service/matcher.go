package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"go.uber.org/zap"
)

type MatchCacheEntry struct {
	Channel   *domain.Channel
	Timestamp time.Time
}

type MemberMatcher struct {
	membersData   domain.MemberDataProvider
	aliasToName   map[string]string
	cache         *CacheService
	holodex       *HolodexService
	geminiService *GeminiService
	logger        *zap.Logger
	matchCache    map[string]*MatchCacheEntry
	matchCacheMu  sync.RWMutex
	matchCacheTTL time.Duration
}

func NewMemberMatcher(
	membersData domain.MemberDataProvider,
	cache *CacheService,
	holodex *HolodexService,
	logger *zap.Logger,
) *MemberMatcher {
	mm := &MemberMatcher{
		membersData:   membersData,
		cache:         cache,
		holodex:       holodex,
		logger:        logger,
		matchCache:    make(map[string]*MatchCacheEntry),
		matchCacheTTL: 1 * time.Minute,
	}

	mm.aliasToName = mm.buildAliasMap()

	logger.Info("MemberMatcher initialized",
		zap.Int("members", len(membersData.GetAllMembers())),
		zap.Int("aliases", len(mm.aliasToName)),
	)

	return mm
}

func (mm *MemberMatcher) SetGeminiService(gemini *GeminiService) {
	mm.geminiService = gemini
	mm.logger.Info("MemberMatcher: Gemini service enabled for smart channel selection")
}

func (mm *MemberMatcher) buildAliasMap() map[string]string {
	aliasMap := make(map[string]string)

	for _, member := range mm.membersData.GetAllMembers() {
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

// tryExactAliasMatch attempts exact match via pre-built alias map
func (mm *MemberMatcher) tryExactAliasMatch(ctx context.Context, queryNorm string) (*domain.Channel, error) {
	englishName, found := mm.aliasToName[queryNorm]
	if !found {
		return nil, nil
	}

	// Try Redis cache first
	if channelID, err := mm.cache.GetMemberChannelID(ctx, englishName); err == nil && channelID != "" {
		if channel, err := mm.holodex.GetChannel(ctx, channelID); err == nil && channel != nil {
			return channel, nil
		}
	}

	// Fallback to static data
	if member := mm.membersData.FindMemberByName(englishName); member != nil {
		if channel, err := mm.holodex.GetChannel(ctx, member.ChannelID); err == nil && channel != nil {
			return channel, nil
		}
	}

	return nil, nil
}

// tryExactRedisMatch attempts exact match in dynamic Redis data
func (mm *MemberMatcher) tryExactRedisMatch(ctx context.Context, query string, dynamicMembers map[string]string) (*domain.Channel, error) {
	for name, channelID := range dynamicMembers {
		if strings.EqualFold(name, query) {
			if channel, err := mm.holodex.GetChannel(ctx, channelID); err == nil && channel != nil {
				return channel, nil
			}
		}
	}
	return nil, nil
}

// tryPartialStaticMatch attempts partial match in static member data
func (mm *MemberMatcher) tryPartialStaticMatch(ctx context.Context, queryNorm string) (*domain.Channel, error) {
	for _, member := range mm.membersData.GetAllMembers() {
		nameNorm := util.Normalize(member.Name)
		if strings.Contains(nameNorm, queryNorm) || strings.Contains(queryNorm, nameNorm) {
			if channel, err := mm.holodex.GetChannel(ctx, member.ChannelID); err == nil && channel != nil {
				return channel, nil
			}
		}
	}
	return nil, nil
}

// tryPartialRedisMatch attempts partial match in dynamic Redis data
func (mm *MemberMatcher) tryPartialRedisMatch(ctx context.Context, queryNorm string, dynamicMembers map[string]string) (*domain.Channel, error) {
	for name, channelID := range dynamicMembers {
		nameNorm := util.Normalize(name)
		if strings.Contains(nameNorm, queryNorm) || strings.Contains(queryNorm, nameNorm) {
			if channel, err := mm.holodex.GetChannel(ctx, channelID); err == nil && channel != nil {
				return channel, nil
			}
		}
	}
	return nil, nil
}

// tryPartialAliasMatch attempts partial match across all aliases
func (mm *MemberMatcher) tryPartialAliasMatch(ctx context.Context, queryNorm string) (*domain.Channel, error) {
	for _, member := range mm.membersData.GetAllMembers() {
		for _, alias := range member.GetAllAliases() {
			aliasNorm := util.Normalize(alias)
			if strings.Contains(aliasNorm, queryNorm) || strings.Contains(queryNorm, aliasNorm) {
				if channel, err := mm.holodex.GetChannel(ctx, member.ChannelID); err == nil && channel != nil {
					return channel, nil
				}
			}
		}
	}
	return nil, nil
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

// selectBestFromCandidates selects best channel from multiple candidates
func (mm *MemberMatcher) selectBestFromCandidates(ctx context.Context, query string, channels []*domain.Channel) (*domain.Channel, error) {
	if len(channels) == 0 {
		return nil, nil
	}

	if len(channels) == 1 {
		mm.logger.Info("Single Holodex result", zap.String("channel", channels[0].Name))
		return channels[0], nil
	}

	// Multiple candidates - use Gemini AI
	if mm.geminiService == nil {
		mm.logger.Info("No Gemini service - using first result", zap.String("channel", channels[0].Name))
		return channels[0], nil
	}

	mm.logger.Info("Gemini AI selection", zap.Int("candidates", len(channels)))

	selected, err := mm.geminiService.SelectBestChannel(ctx, query, channels)
	if err != nil {
		mm.logger.Warn("Gemini selection failed", zap.Error(err))
		return nil, nil
	}

	if selected != nil {
		mm.logger.Info("Gemini selected", zap.String("channel", selected.Name))
		return selected, nil
	}

	mm.logger.Warn("Gemini returned no match")
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

	go func() {
		time.Sleep(mm.matchCacheTTL)
		mm.matchCacheMu.Lock()
		delete(mm.matchCache, cacheKey)
		mm.matchCacheMu.Unlock()
	}()

	return channel, err
}

func (mm *MemberMatcher) findBestMatchImpl(ctx context.Context, query string) (*domain.Channel, error) {
	queryNorm := util.NormalizeSuffix(query)

	// Strategy 1: Exact alias match (fastest)
	if channel, err := mm.tryExactAliasMatch(ctx, queryNorm); err != nil || channel != nil {
		return channel, err
	}

	// Load dynamic members once for strategies 2 & 4
	dynamicMembers := mm.loadDynamicMembers(ctx)

	// Strategy 2: Exact match in Redis
	if channel, err := mm.tryExactRedisMatch(ctx, query, dynamicMembers); err != nil || channel != nil {
		return channel, err
	}

	// Strategy 3: Partial match in static data
	if channel, err := mm.tryPartialStaticMatch(ctx, queryNorm); err != nil || channel != nil {
		return channel, err
	}

	// Strategy 4: Partial match in Redis
	if channel, err := mm.tryPartialRedisMatch(ctx, queryNorm, dynamicMembers); err != nil || channel != nil {
		return channel, err
	}

	// Strategy 5: Partial alias match
	if channel, err := mm.tryPartialAliasMatch(ctx, queryNorm); err != nil || channel != nil {
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
	return mm.membersData.GetAllMembers()
}

func (mm *MemberMatcher) GetMemberByChannelID(channelID string) *domain.Member {
	return mm.membersData.FindMemberByChannelID(channelID)
}
