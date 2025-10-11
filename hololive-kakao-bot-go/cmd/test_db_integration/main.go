package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/service/database"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/member"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
)

func main() {
	logger, _ := util.NewLogger("info", "")
	defer logger.Sync()

	log.Println("=== PostgreSQL Member Data Integration Test ===")
	log.Println()

	// Initialize PostgreSQL
	postgresCfg := database.PostgresConfig{
		Host:     envOrDefault("POSTGRES_HOST", "localhost"),
		Port:     envOrDefaultInt("POSTGRES_PORT", 5432),
		User:     envOrDefault("POSTGRES_USER", "holo_user"),
		Password: envOrDefault("POSTGRES_PASSWORD", "holo_password"),
		Database: envOrDefault("POSTGRES_DB", "holo_oshi_db"),
	}

	postgres, err := database.NewPostgresService(postgresCfg, logger)
	if err != nil {
		log.Fatalf("❌ Failed to connect to PostgreSQL: %v", err)
	}
	defer postgres.Close()
	log.Println("✓ PostgreSQL connected")

	// Initialize Repository
	repo := member.NewMemberRepository(postgres, logger)
	log.Println("✓ MemberRepository created")

	// Test 1: Get all members
	ctx := context.Background()
	members, err := repo.GetAllMembers(ctx)
	if err != nil {
		log.Fatalf("❌ Failed to get all members: %v", err)
	}
	log.Printf("✓ Loaded %d members from PostgreSQL", len(members))

	// Test 2: Find by channel ID
	testChannelID := "UChAnqc_AY5_I3Px5dig3X1Q" // Korone
	foundMember, err := repo.FindByChannelID(ctx, testChannelID)
	if err != nil {
		log.Fatalf("❌ Failed to find by channel ID: %v", err)
	}
	if foundMember == nil {
		log.Fatal("❌ Korone not found!")
	}
	log.Printf("✓ Find by channel ID: %s (aliases: ko=%d, ja=%d)",
		foundMember.Name, len(foundMember.Aliases.Ko), len(foundMember.Aliases.Ja))

	// Test 3: Find by alias
	foundMember, err = repo.FindByAlias(ctx, "코로네")
	if err != nil {
		log.Fatalf("❌ Failed to find by alias: %v", err)
	}
	if foundMember == nil {
		log.Fatal("❌ Alias '코로네' not found!")
	}
	log.Printf("✓ Find by alias '코로네': %s", foundMember.Name)

	// Test 4: Initialize Cache (without Redis)
	memberCache, err := member.NewMemberCache(repo, nil, logger, member.MemberCacheConfig{
		WarmUp:   true,
		RedisTTL: 30 * time.Minute,
	})
	if err != nil {
		log.Fatalf("❌ Failed to create cache: %v", err)
	}
	log.Println("✓ MemberCache created with warm-up")

	// Test 5: Cache queries
	foundMember, err = memberCache.GetByChannelID(ctx, testChannelID)
	if err != nil {
		log.Fatalf("❌ Cache GetByChannelID failed: %v", err)
	}
	if foundMember == nil {
		log.Fatal("❌ Korone not in cache!")
	}
	log.Printf("✓ Cache hit: %s", foundMember.Name)

	// Test 6: Adapter
	adapter := member.NewMemberServiceAdapter(memberCache)
	adapterCtx := adapter.WithContext(ctx)
	foundMember = adapterCtx.FindMemberByChannelID(testChannelID)
	if foundMember == nil {
		log.Fatal("❌ Adapter failed!")
	}
	log.Printf("✓ Adapter works: %s", foundMember.Name)

	channelIDs := adapterCtx.GetChannelIDs()
	log.Printf("✓ Adapter GetChannelIDs: %d channels", len(channelIDs))

	allMembers := adapterCtx.GetAllMembers()
	log.Printf("✓ Adapter GetAllMembers: %d members", len(allMembers))

	log.Println()
	log.Println("=== ✅ ALL TESTS PASSED ===")
	log.Println()
	fmt.Println("Summary:")
	fmt.Printf("- Total members: %d\n", len(members))
	fmt.Printf("- With channel ID: %d\n", len(channelIDs))
	fmt.Printf("- Repository: ✓ Working\n")
	fmt.Printf("- Cache: ✓ Working\n")
	fmt.Printf("- Adapter: ✓ Working\n")
	fmt.Printf("- Alias search: ✓ Working\n")
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envOrDefaultInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("⚠ Invalid value for %s (%s), using default %d\n", key, value, fallback)
		return fallback
	}
	return parsed
}
