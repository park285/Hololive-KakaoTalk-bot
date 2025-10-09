package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/service"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
)

func main() {
	logger, _ := util.NewLogger("info", "")
	defer logger.Sync()

	log.Println("=== PostgreSQL Member Data Integration Test ===")
	log.Println()

	// Initialize PostgreSQL
	postgres, err := service.NewPostgresService(service.PostgresConfig{
		Host:     "localhost",
		Port:     5433,
		User:     "holo_user",
		Password: "holo_password",
		Database: "holo_oshi_db",
	}, logger)
	if err != nil {
		log.Fatalf("❌ Failed to connect to PostgreSQL: %v", err)
	}
	defer postgres.Close()
	log.Println("✓ PostgreSQL connected")

	// Initialize Repository
	repo := service.NewMemberRepository(postgres, logger)
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
	member, err := repo.FindByChannelID(ctx, testChannelID)
	if err != nil {
		log.Fatalf("❌ Failed to find by channel ID: %v", err)
	}
	if member == nil {
		log.Fatal("❌ Korone not found!")
	}
	log.Printf("✓ Find by channel ID: %s (aliases: ko=%d, ja=%d)",
		member.Name, len(member.Aliases.Ko), len(member.Aliases.Ja))

	// Test 3: Find by alias
	member, err = repo.FindByAlias(ctx, "코로네")
	if err != nil {
		log.Fatalf("❌ Failed to find by alias: %v", err)
	}
	if member == nil {
		log.Fatal("❌ Alias '코로네' not found!")
	}
	log.Printf("✓ Find by alias '코로네': %s", member.Name)

	// Test 4: Initialize Cache (without Redis)
	cache, err := service.NewMemberCache(repo, nil, logger, service.MemberCacheConfig{
		WarmUp:   true,
		RedisTTL: 30 * time.Minute,
	})
	if err != nil {
		log.Fatalf("❌ Failed to create cache: %v", err)
	}
	log.Println("✓ MemberCache created with warm-up")

	// Test 5: Cache queries
	member, err = cache.GetByChannelID(ctx, testChannelID)
	if err != nil {
		log.Fatalf("❌ Cache GetByChannelID failed: %v", err)
	}
	if member == nil {
		log.Fatal("❌ Korone not in cache!")
	}
	log.Printf("✓ Cache hit: %s", member.Name)

	// Test 6: Adapter
	adapter := service.NewMemberServiceAdapter(cache)
	member = adapter.FindMemberByChannelID(testChannelID)
	if member == nil {
		log.Fatal("❌ Adapter failed!")
	}
	log.Printf("✓ Adapter works: %s", member.Name)

	channelIDs := adapter.GetChannelIDs()
	log.Printf("✓ Adapter GetChannelIDs: %d channels", len(channelIDs))

	allMembers := adapter.GetAllMembers()
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
