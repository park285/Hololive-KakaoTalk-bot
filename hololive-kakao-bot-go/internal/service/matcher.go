package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
)

// MatchCacheEntry holds cached match results
type MatchCacheEntry struct {
	Channel   *domain.Channel
	Timestamp time.Time
}

// MemberMatcher matches user queries to Hololive members
type MemberMatcher struct {
	membersData   *domain.MembersData
	aliasToName   map[string]string
	cache         *CacheService
	holodex       *HolodexService
	geminiService *GeminiService
	logger        *zap.Logger
	matchCache    map[string]*MatchCacheEntry
	matchCacheMu  sync.RWMutex
	matchCacheTTL time.Duration
}

// NewMemberMatcher creates a new MemberMatcher
func NewMemberMatcher(
	membersData *domain.MembersData,
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

	// Build alias map
	mm.aliasToName = mm.buildAliasMap()

	logger.Info("MemberMatcher initialized",
		zap.Int("members", len(membersData.Members)),
		zap.Int("aliases", len(mm.aliasToName)),
	)

	return mm
}

// SetGeminiService sets the Gemini service for smart channel selection
func (mm *MemberMatcher) SetGeminiService(gemini *GeminiService) {
	mm.geminiService = gemini
	mm.logger.Info("MemberMatcher: Gemini service enabled for smart channel selection")
}

// buildAliasMap builds a map from alias to English name
func (mm *MemberMatcher) buildAliasMap() map[string]string {
	aliasMap := make(map[string]string)

	for _, member := range mm.membersData.Members {
		// Add English name
		aliasMap[strings.ToLower(member.Name)] = member.Name

		// Add Japanese name if exists
		if member.NameJa != "" {
			aliasMap[strings.ToLower(member.NameJa)] = member.Name
		}

		// Add Korean aliases
		if member.Aliases != nil && len(member.Aliases.Ko) > 0 {
			for _, alias := range member.Aliases.Ko {
				aliasMap[strings.ToLower(alias)] = member.Name
			}
		}

		// Add Japanese aliases
		if member.Aliases != nil && len(member.Aliases.Ja) > 0 {
			for _, alias := range member.Aliases.Ja {
				aliasMap[strings.ToLower(alias)] = member.Name
			}
		}
	}

	return aliasMap
}

// FindBestMatch finds the best matching channel for a query
// Matching strategy:
// 1. Exact alias match (static data) → Redis lookup
// 2. Redis full search (dynamically added members)
// 3. Partial string matching (fallback)
// 4. Holodex API search
func (mm *MemberMatcher) FindBestMatch(ctx context.Context, query string) (*domain.Channel, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	cacheKey := fmt.Sprintf("match:%s", normalizedQuery)

	// Check cache
	mm.matchCacheMu.RLock()
	cached, found := mm.matchCache[cacheKey]
	mm.matchCacheMu.RUnlock()

	if found {
		age := time.Since(cached.Timestamp)
		if age < mm.matchCacheTTL {
			mm.logger.Debug("Match cache hit",
				zap.String("query", query),
				zap.Duration("age", age),
			)
			return cached.Channel, nil
		}

		// Expired
		mm.matchCacheMu.Lock()
		delete(mm.matchCache, cacheKey)
		mm.matchCacheMu.Unlock()
	}

	// Fresh match
	mm.logger.Debug("Match cache miss", zap.String("query", query))
	channel, err := mm.findBestMatchImpl(ctx, query)

	// Cache result (even if nil)
	mm.matchCacheMu.Lock()
	mm.matchCache[cacheKey] = &MatchCacheEntry{
		Channel:   channel,
		Timestamp: time.Now(),
	}
	mm.matchCacheMu.Unlock()

	// Auto cleanup
	go func() {
		time.Sleep(mm.matchCacheTTL)
		mm.matchCacheMu.Lock()
		delete(mm.matchCache, cacheKey)
		mm.matchCacheMu.Unlock()
	}()

	return channel, err
}

// findBestMatchImpl implements the actual matching logic
func (mm *MemberMatcher) findBestMatchImpl(ctx context.Context, query string) (*domain.Channel, error) {
	queryLower := strings.ToLower(strings.TrimSpace(query))

	// Remove "짱" suffix
	if strings.HasSuffix(queryLower, "짱") {
		original := queryLower
		queryLower = queryLower[:len(queryLower)-len("짱")]
		mm.logger.Debug("Normalized suffix",
			zap.String("original", original),
			zap.String("normalized", queryLower),
		)
	}

	// 1. Exact alias match
	if englishName, found := mm.aliasToName[queryLower]; found {
		mm.logger.Debug("Alias match", zap.String("query", query), zap.String("english_name", englishName))

		if channelID, err := mm.cache.GetMemberChannelID(ctx, englishName); err == nil && channelID != "" {
			channel, err := mm.holodex.GetChannel(ctx, channelID)
			if err == nil && channel != nil {
				mm.logger.Debug("Channel found via Redis member cache",
					zap.String("english_name", englishName),
					zap.String("channel_id", channelID),
				)
				return channel, nil
			}
		} else if err != nil {
			mm.logger.Warn("Failed to get member channel ID from cache",
				zap.String("member", englishName),
				zap.Error(err),
			)
		}

		// Try static data first
		member := mm.membersData.FindMemberByName(englishName)
		if member != nil {
			channel, err := mm.holodex.GetChannel(ctx, member.ChannelID)
			if err == nil && channel != nil {
				mm.logger.Debug("Channel found via alias",
					zap.String("english_name", englishName),
					zap.String("channel_id", member.ChannelID),
				)
				return channel, nil
			}
		}
	}

	// Load dynamic member map lazily
	var dynamicMembers map[string]string
	var dynamicLoaded bool
	loadDynamicMembers := func() map[string]string {
		if dynamicLoaded {
			return dynamicMembers
		}
		dynamicLoaded = true
		members, err := mm.cache.GetAllMembers(ctx)
		if err != nil {
			mm.logger.Warn("Failed to load members from cache", zap.Error(err))
			dynamicMembers = map[string]string{}
		} else {
			dynamicMembers = members
		}
		return dynamicMembers
	}

	// 2. Exact match in dynamic member map
	if membersMap := loadDynamicMembers(); len(membersMap) > 0 {
		for name, channelID := range membersMap {
			if strings.EqualFold(name, query) {
				channel, err := mm.holodex.GetChannel(ctx, channelID)
				if err == nil && channel != nil {
					mm.logger.Debug("Exact match in Redis member map",
						zap.String("member", name),
						zap.String("channel_id", channelID),
					)
					return channel, nil
				}
			}
		}
	}

	// 3. Partial string matching (static data)
	for _, member := range mm.membersData.Members {
		nameLower := strings.ToLower(member.Name)
		if strings.Contains(nameLower, queryLower) || strings.Contains(queryLower, nameLower) {
			mm.logger.Debug("Partial match (English name bidirectional)",
				zap.String("member", member.Name),
				zap.String("query", query),
			)

			channel, err := mm.holodex.GetChannel(ctx, member.ChannelID)
			if err == nil && channel != nil {
				return channel, nil
			}
		}
	}

	// Partial match against dynamic members if available
	if membersMap := loadDynamicMembers(); len(membersMap) > 0 {
		for name, channelID := range membersMap {
			nameLower := strings.ToLower(name)
			if strings.Contains(nameLower, queryLower) || strings.Contains(queryLower, nameLower) {
				channel, err := mm.holodex.GetChannel(ctx, channelID)
				if err == nil && channel != nil {
					mm.logger.Debug("Partial match in Redis member map",
						zap.String("member", name),
						zap.String("channel_id", channelID),
					)
					return channel, nil
				}
			}
		}
	}

	// 4. Partial alias matching
	for _, member := range mm.membersData.Members {
		allAliases := member.GetAllAliases()
		for _, alias := range allAliases {
			aliasLower := strings.ToLower(alias)
			if strings.Contains(aliasLower, queryLower) || strings.Contains(queryLower, aliasLower) {
				mm.logger.Debug("Partial match (alias bidirectional)",
					zap.String("alias", alias),
					zap.String("query", query),
					zap.String("member", member.Name),
				)

				channel, err := mm.holodex.GetChannel(ctx, member.ChannelID)
				if err == nil && channel != nil {
					return channel, nil
				}
			}
		}
	}

	// Holodex API search (last resort)
	mm.logger.Info("Fallback: Searching Holodex API", zap.String("query", query))

	channels, err := mm.holodex.SearchChannels(ctx, query)
	if err != nil {
		mm.logger.Warn("Holodex API search failed", zap.String("query", query), zap.Error(err))
		return nil, nil
	}

	if len(channels) == 0 {
		mm.logger.Warn("No results from Holodex API", zap.String("query", query))
		return nil, nil
	}

	// Use Gemini for smart selection if available
	if mm.geminiService != nil && len(channels) > 1 {
		mm.logger.Info("Using Gemini to select best match", zap.Int("candidates", len(channels)))

		selected, err := mm.geminiService.SelectBestChannel(ctx, query, channels)
		if err != nil {
			mm.logger.Warn("Gemini selection failed", zap.Error(err))
			return nil, nil
		}

		if selected != nil {
			mm.logger.Info("Gemini selected", zap.String("channel", selected.Name))
			return selected, nil
		}

		mm.logger.Warn("Gemini selection returned no match")
		return nil, nil
	}

	// Return first result
	channel := channels[0]
	if channel != nil {
		mm.logger.Info("Found via Holodex API", zap.String("channel", channel.Name))
	}

	return channel, nil
}

// GetAllMembers returns all members
func (mm *MemberMatcher) GetAllMembers() []*domain.Member {
	return mm.membersData.Members
}

// GetMemberByChannelID returns a member by channel ID
func (mm *MemberMatcher) GetMemberByChannelID(channelID string) *domain.Member {
	return mm.membersData.FindMemberByChannelID(channelID)
}
