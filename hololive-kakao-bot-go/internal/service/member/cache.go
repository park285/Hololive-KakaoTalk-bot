package member

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type MemberCache struct {
	repo   *MemberRepository
	redis  *redis.Client
	logger *zap.Logger

	// In-memory caches
	byChannelID sync.Map // map[string]*domain.Member
	byName      sync.Map // map[string]*domain.Member
	allMembers  sync.Map // []string (channel IDs)

	// Cache configuration
	redisTTL time.Duration
	warmup   bool
}

type MemberCacheConfig struct {
	RedisTTL time.Duration
	WarmUp   bool // Load all members into memory on startup
}

func NewMemberCache(repo *MemberRepository, redis *redis.Client, logger *zap.Logger, cfg MemberCacheConfig) (*MemberCache, error) {
	if cfg.RedisTTL == 0 {
		cfg.RedisTTL = 30 * time.Minute
	}

	cache := &MemberCache{
		repo:     repo,
		redis:    redis,
		logger:   logger,
		redisTTL: cfg.RedisTTL,
		warmup:   cfg.WarmUp,
	}

	// Warm up cache if enabled
	if cfg.WarmUp {
		if err := cache.WarmUpCache(context.Background()); err != nil {
			logger.Warn("Failed to warm up cache", zap.Error(err))
		}
	}

	return cache, nil
}

func (c *MemberCache) WarmUpCache(ctx context.Context) error {
	members, err := c.repo.GetAllMembers(ctx)
	if err != nil {
		return fmt.Errorf("failed to load all members: %w", err)
	}

	for _, member := range members {
		if member.ChannelID != "" {
			c.byChannelID.Store(member.ChannelID, member)
		}
		c.byName.Store(member.Name, member)
	}

	c.logger.Info("Member cache warmed up",
		zap.Int("total_members", len(members)),
	)

	return nil
}

func (c *MemberCache) GetByChannelID(ctx context.Context, channelID string) (*domain.Member, error) {
	if val, ok := c.byChannelID.Load(channelID); ok {
		return val.(*domain.Member), nil
	}

	if c.redis != nil {
		cacheKey := fmt.Sprintf("member:channel:%s", channelID)
		data, err := c.redis.Get(ctx, cacheKey).Bytes()
		if err == nil {
			var member domain.Member
			if err := json.Unmarshal(data, &member); err == nil {
				// Store in memory for next access
				c.byChannelID.Store(channelID, &member)
				return &member, nil
			}
		}
	}

	member, err := c.repo.FindByChannelID(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, nil
	}

	// Cache the result
	c.cacheMember(ctx, member)

	return member, nil
}

func (c *MemberCache) GetByName(ctx context.Context, name string) (*domain.Member, error) {
	if val, ok := c.byName.Load(name); ok {
		return val.(*domain.Member), nil
	}

	if c.redis != nil {
		cacheKey := fmt.Sprintf("member:name:%s", name)
		data, err := c.redis.Get(ctx, cacheKey).Bytes()
		if err == nil {
			var member domain.Member
			if err := json.Unmarshal(data, &member); err == nil {
				c.byName.Store(name, &member)
				return &member, nil
			}
		}
	}

	member, err := c.repo.FindByName(ctx, name)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, nil
	}

	c.cacheMember(ctx, member)
	return member, nil
}

func (c *MemberCache) FindByAlias(ctx context.Context, alias string) (*domain.Member, error) {

	if c.redis != nil {
		cacheKey := fmt.Sprintf("member:alias:%s", alias)
		data, err := c.redis.Get(ctx, cacheKey).Bytes()
		if err == nil {
			var member domain.Member
			if err := json.Unmarshal(data, &member); err == nil {
				// Cache by channel and name for future lookups
				if member.ChannelID != "" {
					c.byChannelID.Store(member.ChannelID, &member)
				}
				c.byName.Store(member.Name, &member)
				return &member, nil
			}
		}
	}

	member, err := c.repo.FindByAlias(ctx, alias)
	if err != nil {
		return nil, err
	}
	if member == nil {
		return nil, nil
	}

	c.cacheMember(ctx, member)

	if c.redis != nil {
		aliasKey := fmt.Sprintf("member:alias:%s", alias)
		data, _ := json.Marshal(member)
		c.redis.Set(ctx, aliasKey, data, c.redisTTL)
	}

	return member, nil
}

func (c *MemberCache) GetAllChannelIDs(ctx context.Context) ([]string, error) {
	if val, ok := c.allMembers.Load("channel_ids"); ok {
		return val.([]string), nil
	}

	channelIDs, err := c.repo.GetAllChannelIDs(ctx)
	if err != nil {
		return nil, err
	}

	c.allMembers.Store("channel_ids", channelIDs)

	return channelIDs, nil
}

func (c *MemberCache) cacheMember(ctx context.Context, member *domain.Member) {
	if member.ChannelID != "" {
		c.byChannelID.Store(member.ChannelID, member)
	}
	c.byName.Store(member.Name, member)

	if c.redis != nil {
		data, err := json.Marshal(member)
		if err != nil {
			c.logger.Warn("Failed to marshal member for cache", zap.Error(err))
			return
		}

		if member.ChannelID != "" {
			c.redis.Set(ctx, fmt.Sprintf("member:channel:%s", member.ChannelID), data, c.redisTTL)
		}
		c.redis.Set(ctx, fmt.Sprintf("member:name:%s", member.Name), data, c.redisTTL)
	}
}

func (c *MemberCache) InvalidateAll(ctx context.Context) error {
	c.byChannelID = sync.Map{}
	c.byName = sync.Map{}
	c.allMembers = sync.Map{}

	if c.redis != nil {
		pattern := "member:*"
		iter := c.redis.Scan(ctx, 0, pattern, 0).Iterator()
		for iter.Next(ctx) {
			c.redis.Del(ctx, iter.Val())
		}
		if err := iter.Err(); err != nil {
			return fmt.Errorf("failed to invalidate redis cache: %w", err)
		}
	}

	c.logger.Info("Member cache invalidated")
	return nil
}

func (c *MemberCache) Refresh(ctx context.Context) error {
	if err := c.InvalidateAll(ctx); err != nil {
		return err
	}
	return c.WarmUpCache(ctx)
}
