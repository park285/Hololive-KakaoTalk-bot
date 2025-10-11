package ai

import (
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

type ParseCacheEntry struct {
	Result    *domain.ParseResults
	Metadata  *GenerateMetadata
	Timestamp time.Time
}

type ParseCache struct {
	mu      sync.RWMutex
	entries map[string]*ParseCacheEntry
	ttl     time.Duration
}

func NewParseCache(ttl time.Duration) *ParseCache {
	return &ParseCache{
		entries: make(map[string]*ParseCacheEntry),
		ttl:     ttl,
	}
}

func (c *ParseCache) Get(key string) (*ParseCacheEntry, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}

	if time.Since(entry.Timestamp) >= c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}

	return entry, true
}

func (c *ParseCache) Set(key string, result *domain.ParseResults, metadata *GenerateMetadata) {
	c.mu.Lock()
	c.entries[key] = &ParseCacheEntry{
		Result:    result,
		Metadata:  metadata,
		Timestamp: time.Now(),
	}
	c.mu.Unlock()
}

func (c *ParseCache) Clear(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}
