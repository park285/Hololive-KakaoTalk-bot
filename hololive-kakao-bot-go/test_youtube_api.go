// +build ignore

package main

import (
	"context"
	"fmt"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/config"
	"github.com/kapu/hololive-kakao-bot-go/internal/service"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Load config
	cfg, err := config.Load()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	if cfg.YouTube.APIKey == "" {
		fmt.Println("❌ YOUTUBE_API_KEY not set in .env")
		return
	}

	fmt.Printf("Using API Key: %s...%s\n\n",
		cfg.YouTube.APIKey[:10],
		cfg.YouTube.APIKey[len(cfg.YouTube.APIKey)-4:])

	// Initialize cache
	cache, err := service.NewCacheService(service.CacheConfig{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}, logger)
	if err != nil {
		logger.Fatal("Failed to create cache", zap.Error(err))
	}

	// Create YouTube service
	youtube, err := service.NewYouTubeService(cfg.YouTube.APIKey, cache, logger)
	if err != nil {
		logger.Fatal("Failed to create YouTube service", zap.Error(err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test 1: Get channel statistics (Pekora)
	fmt.Println("=== Test 1: Channel Statistics ===")
	pekoraID := "UC1DCedRgGHBdm81E1llLhOQ"

	stats, err := youtube.GetChannelStatistics(ctx, []string{pekoraID})
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		if pekStats, found := stats[pekoraID]; found {
			fmt.Printf("✅ %s\n", pekStats.ChannelTitle)
			fmt.Printf("   Subscribers: %s\n", formatNumber(pekStats.SubscriberCount))
			fmt.Printf("   Videos: %s\n", formatNumber(pekStats.VideoCount))
			fmt.Printf("   Views: %s\n", formatNumber(pekStats.ViewCount))
		}
	}

	// Test 2: Batch statistics (multiple members)
	fmt.Println("\n=== Test 2: Batch Statistics (5 members) ===")
	testChannels := []string{
		"UC1DCedRgGHBdm81E1llLhOQ", // Pekora
		"UC-hM6YJuNYVAmUWxeIr9FeA", // Miko
		"UCdn5BQ06XqgXoAxIhbqw5Rg", // Fubuki
		"UC1uv2Oq6kNxgATlCiez59hw", // Towa
		"UCvInZx9h3jC2JzsIzoOebWg", // Flare
	}

	batchStats, err := youtube.GetChannelStatistics(ctx, testChannels)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Printf("✅ Fetched %d channels\n", len(batchStats))
		for _, channelID := range testChannels {
			if s, found := batchStats[channelID]; found {
				fmt.Printf("   %s: %s subs\n", s.ChannelTitle, formatNumber(s.SubscriberCount))
			}
		}
	}

	// Test 3: Recent videos
	fmt.Println("\n=== Test 3: Recent Videos ===")
	videos, err := youtube.GetRecentVideos(ctx, pekoraID, 5)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
	} else {
		fmt.Printf("✅ Found %d recent videos\n", len(videos))
		for i, vid := range videos {
			fmt.Printf("   %d. https://youtube.com/watch?v=%s\n", i+1, vid)
		}
	}

	// Test 4: Quota usage
	used, remaining, resetTime := youtube.GetQuotaStatus()
	fmt.Println("\n=== Quota Usage ===")
	fmt.Printf("Used: %d units\n", used)
	fmt.Printf("Remaining: %d units\n", remaining)
	fmt.Printf("Resets at: %s\n", resetTime.Format("2006-01-02 15:04 MST"))

	fmt.Println("\n✅ All tests completed!")
}

func formatNumber(n uint64) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
