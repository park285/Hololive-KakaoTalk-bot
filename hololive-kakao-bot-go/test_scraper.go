//go:build ignore
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
	// Initialize logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Load config
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Fatal("Failed to load config", zap.Error(err))
	}

	// Initialize Redis cache
	cache, err := service.NewCacheService(service.CacheConfig{
		RedisAddr:     fmt.Sprintf("%s:%d", cfg.Redis.Host, cfg.Redis.Port),
		RedisPassword: cfg.Redis.Password,
		RedisDB:       cfg.Redis.DB,
	}, logger)
	if err != nil {
		logger.Fatal("Failed to create cache", zap.Error(err))
	}

	// Create scraper
	scraper := service.NewScraperService(cache, logger)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Test 1: Fetch all streams
	fmt.Println("\n=== Test 1: Fetching all streams ===")
	allStreams, err := scraper.FetchAllStreams(ctx)
	if err != nil {
		logger.Error("Failed to fetch all streams", zap.Error(err))
	} else {
		fmt.Printf("✅ Total streams found: %d\n", len(allStreams))

		// Show first 5 streams
		for i, stream := range allStreams {
			if i >= 5 {
				break
			}
			fmt.Printf("\nStream #%d:\n", i+1)
			fmt.Printf("  ID: %s\n", stream.ID)
			fmt.Printf("  Title: %s\n", stream.Title)
			fmt.Printf("  Channel: %s (%s)\n", stream.ChannelName, stream.ChannelID)
			if stream.StartScheduled != nil {
				fmt.Printf("  Start: %s\n", stream.StartScheduled.Format("2006-01-02 15:04"))
			}
			if stream.Link != nil {
				fmt.Printf("  URL: %s\n", *stream.Link)
			}
		}
	}

	// Test 2: Fetch specific channel (Pekora)
	fmt.Println("\n\n=== Test 2: Fetching Pekora's schedule ===")
	pekoraChannelID := "UC1DCedRgGHBdm81E1llLhOQ"

	pekoraStreams, err := scraper.FetchChannel(ctx, pekoraChannelID)
	if err != nil {
		logger.Error("Failed to fetch Pekora's schedule", zap.Error(err))
	} else {
		fmt.Printf("✅ Pekora streams found: %d\n", len(pekoraStreams))

		for i, stream := range pekoraStreams {
			fmt.Printf("\nPekora Stream #%d:\n", i+1)
			fmt.Printf("  ID: %s\n", stream.ID)
			fmt.Printf("  Title: %s\n", stream.Title)
			if stream.StartScheduled != nil {
				fmt.Printf("  Start: %s\n", stream.StartScheduled.Format("2006-01-02 15:04"))
			}
		}
	}

	// Test 3: Structure validation
	fmt.Println("\n\n=== Test 3: HTML structure validation ===")
	if err := scraper.ValidateStructure(ctx); err != nil {
		logger.Error("❌ Structure validation failed", zap.Error(err))
	} else {
		fmt.Println("✅ HTML structure is valid")
	}

	fmt.Println("\n=== All tests completed ===")
}
