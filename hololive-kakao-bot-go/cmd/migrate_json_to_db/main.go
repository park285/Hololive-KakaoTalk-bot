package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

// CLI flags
var (
	dryRun   = flag.Bool("dry-run", false, "Run without committing to database")
	dbHost   = flag.String("db-host", "localhost", "PostgreSQL host")
	dbPort   = flag.Int("db-port", 5433, "PostgreSQL port")
	dbUser   = flag.String("db-user", "holo_user", "PostgreSQL user")
	dbPass   = flag.String("db-pass", "holo_password", "PostgreSQL password")
	dbName   = flag.String("db-name", "holo_oshi_db", "PostgreSQL database")
	verbose  = flag.Bool("verbose", false, "Verbose output")
)

// Data structures matching JSON files
type OfficialTalent struct {
	Japanese string `json:"japanese"`
	English  string `json:"english"`
	Link     string `json:"link"`
	Status   string `json:"status"` // active, graduated, ended
}

type Member struct {
	ChannelID   string   `json:"channelId"`
	Name        string   `json:"name"`
	Aliases     *Aliases `json:"aliases,omitempty"`
	NameJa      string   `json:"nameJa,omitempty"`
	NameKo      string   `json:"nameKo,omitempty"`
	IsGraduated bool     `json:"isGraduated,omitempty"`
}

type Aliases struct {
	Ko []string `json:"ko,omitempty"`
	Ja []string `json:"ja,omitempty"`
}

type RawProfile struct {
	Slug         string              `json:"slug"`
	EnglishName  string              `json:"english_name"`
	JapaneseName string              `json:"japanese_name"`
	Catchphrase  string              `json:"catchphrase"`
	Description  string              `json:"description"`
	DataEntries  []map[string]string `json:"data_entries"`
}

type TranslatedProfile struct {
	DisplayName string              `json:"display_name"`
	Catchphrase string              `json:"catchphrase"`
	Summary     string              `json:"summary"`
	Highlights  []string            `json:"highlights"`
	Data        []map[string]string `json:"data"`
}

// Unified member data for insertion
type UnifiedMember struct {
	Slug         string
	ChannelID    string
	EnglishName  string
	JapaneseName string
	KoreanName   string
	Status       string
	IsGraduated  bool
	OfficialLink string
	Aliases      *Aliases
	RawProfile   *RawProfile
	KoProfile    *TranslatedProfile
}

func main() {
	flag.Parse()

	log.Println("===========================")
	log.Println("JSON to PostgreSQL Migration")
	log.Println("===========================")

	if *dryRun {
		log.Println("[DRY RUN MODE] No database changes will be made")
	}

	// Step 1: Load all JSON data
	talents, err := loadOfficialTalents()
	if err != nil {
		log.Fatalf("Failed to load official_talents.json: %v", err)
	}
	log.Printf("✓ Loaded %d official talents", len(talents))

	members, err := loadMembers()
	if err != nil {
		log.Fatalf("Failed to load members.json: %v", err)
	}
	log.Printf("✓ Loaded %d members from members.json", len(members))

	rawProfiles, err := loadRawProfiles()
	if err != nil {
		log.Fatalf("Failed to load raw profiles: %v", err)
	}
	log.Printf("✓ Loaded %d raw profiles", len(rawProfiles))

	koProfiles, err := loadTranslatedProfiles()
	if err != nil {
		log.Fatalf("Failed to load Korean profiles: %v", err)
	}
	log.Printf("✓ Loaded %d Korean profiles", len(koProfiles))

	// Step 2: Unify data sources
	unified := unifyData(talents, members, rawProfiles, koProfiles)
	log.Printf("✓ Unified into %d member records", len(unified))

	// Step 3: Validate data
	if err := validateUnifiedData(unified); err != nil {
		log.Fatalf("Data validation failed: %v", err)
	}
	log.Println("✓ Data validation passed")

	// Step 4: Connect to database
	if !*dryRun {
		db, err := connectDB()
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}
		defer db.Close()

		// Step 5: Insert data
		if err := insertData(db, unified); err != nil {
			log.Fatalf("Failed to insert data: %v", err)
		}

		log.Println("✓ Migration completed successfully")
	} else {
		log.Println("✓ Dry-run completed successfully")
		printSummary(unified)
	}
}

func loadOfficialTalents() ([]OfficialTalent, error) {
	data, err := os.ReadFile("internal/domain/data/official_talents.json")
	if err != nil {
		return nil, err
	}

	var talents []OfficialTalent
	if err := json.Unmarshal(data, &talents); err != nil {
		return nil, err
	}

	return talents, nil
}

func loadMembers() (map[string]*Member, error) {
	data, err := os.ReadFile("internal/domain/data/members.json")
	if err != nil {
		return nil, err
	}

	var doc struct {
		Members []*Member `json:"members"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}

	byName := make(map[string]*Member)
	for _, m := range doc.Members {
		byName[strings.ToLower(m.Name)] = m
	}

	return byName, nil
}

func loadRawProfiles() (map[string]*RawProfile, error) {
	profiles := make(map[string]*RawProfile)
	dir := "internal/domain/data/official_profiles_raw"

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		slug := strings.TrimSuffix(file.Name(), ".json")
		path := filepath.Join(dir, file.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}

		var profile RawProfile
		if err := json.Unmarshal(data, &profile); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}

		if profile.Slug == "" {
			profile.Slug = slug
		}

		profiles[slug] = &profile
	}

	return profiles, nil
}

func loadTranslatedProfiles() (map[string]*TranslatedProfile, error) {
	profiles := make(map[string]*TranslatedProfile)
	dir := "internal/domain/data/official_profiles_ko"

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".json") {
			continue
		}

		slug := strings.TrimSuffix(file.Name(), ".json")
		path := filepath.Join(dir, file.Name())

		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", path, err)
		}

		var profile TranslatedProfile
		if err := json.Unmarshal(data, &profile); err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", path, err)
		}

		profiles[slug] = &profile
	}

	return profiles, nil
}

func unifyData(
	talents []OfficialTalent,
	members map[string]*Member,
	rawProfiles map[string]*RawProfile,
	koProfiles map[string]*TranslatedProfile,
) []*UnifiedMember {
	unified := make([]*UnifiedMember, 0, len(talents))

	for _, talent := range talents {
		// Create slug from talent.Link or talent.English
		slug := extractSlug(talent.Link, talent.English)

		member := members[strings.ToLower(talent.English)]

		um := &UnifiedMember{
			Slug:         slug,
			EnglishName:  talent.English,
			JapaneseName: talent.Japanese,
			Status:       normalizeStatus(talent.Status),
			OfficialLink: talent.Link,
			RawProfile:   rawProfiles[slug],
			KoProfile:    koProfiles[slug],
		}

		// Merge data from members.json
		if member != nil {
			um.ChannelID = member.ChannelID
			um.KoreanName = member.NameKo
			um.Aliases = member.Aliases
			if member.IsGraduated {
				um.IsGraduated = true
			}
		}

		unified = append(unified, um)
	}

	return unified
}

func extractSlug(link, englishName string) string {
	if link != "" {
		parts := strings.Split(strings.Trim(link, "/"), "/")
		if len(parts) > 0 {
			slug := parts[len(parts)-1]
			if slug != "" {
				return slug
			}
		}
	}

	// Fallback: create slug from english name
	slug := strings.ToLower(englishName)
	slug = strings.ReplaceAll(slug, " ", "-")
	slug = strings.ReplaceAll(slug, "'", "")
	return slug
}

func normalizeStatus(status string) string {
	status = strings.ToLower(strings.TrimSpace(status))
	if strings.Contains(status, "graduated") {
		return "graduated"
	}
	if strings.Contains(status, "ended") {
		return "ended"
	}
	return "active"
}

func validateUnifiedData(unified []*UnifiedMember) error {
	slugs := make(map[string]bool)
	channelIDs := make(map[string]bool)

	for i, um := range unified {
		// Check required fields
		if um.Slug == "" {
			return fmt.Errorf("member %d: missing slug", i)
		}
		if um.EnglishName == "" {
			return fmt.Errorf("member %d (%s): missing english name", i, um.Slug)
		}

		// Check uniqueness
		if slugs[um.Slug] {
			return fmt.Errorf("duplicate slug: %s", um.Slug)
		}
		slugs[um.Slug] = true

		if um.ChannelID != "" {
			if channelIDs[um.ChannelID] {
				return fmt.Errorf("duplicate channel_id: %s (%s)", um.ChannelID, um.Slug)
			}
			channelIDs[um.ChannelID] = true
		}
	}

	return nil
}

func connectDB() (*sql.DB, error) {
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		*dbHost, *dbPort, *dbUser, *dbPass, *dbName)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func insertData(db *sql.DB, unified []*UnifiedMember) error {
	ctx := context.Background()

	// Begin transaction
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert members
	for _, um := range unified {
		if err := insertMember(ctx, tx, um); err != nil {
			return fmt.Errorf("failed to insert member %s: %w", um.EnglishName, err)
		}

		if *verbose {
			log.Printf("  → Inserted: %s (%s)", um.EnglishName, um.Slug)
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("✓ Inserted %d members with profiles", len(unified))
	return nil
}

func insertMember(ctx context.Context, tx *sql.Tx, um *UnifiedMember) error {
	// Prepare aliases JSONB  
	var aliasesJSON []byte
	var err error
	if um.Aliases != nil {
		// Manually construct JSON to avoid omitempty issues
		koJSON, _ := json.Marshal(um.Aliases.Ko)
		jaJSON, _ := json.Marshal(um.Aliases.Ja)
		if koJSON == nil || string(koJSON) == "null" {
			koJSON = []byte("[]")
		}
		if jaJSON == nil || string(jaJSON) == "null" {
			jaJSON = []byte("[]")
		}
		aliasesJSON = []byte(fmt.Sprintf(`{"ko": %s, "ja": %s}`, koJSON, jaJSON))
	} else {
		aliasesJSON = []byte(`{"ko": [], "ja": []}`)
	}

	// Debug logging
	if *verbose && um.EnglishName == "Gigi Murin" {
		log.Printf("DEBUG Gigi Murin aliases JSON: %s", string(aliasesJSON))
	}

	// Insert member
	var memberID int
	err = tx.QueryRowContext(ctx, `
		INSERT INTO members (slug, channel_id, english_name, japanese_name, korean_name, status, is_graduated, official_link, aliases)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, um.Slug, nullString(um.ChannelID), um.EnglishName, nullString(um.JapaneseName), nullString(um.KoreanName),
		um.Status, um.IsGraduated, nullString(um.OfficialLink), aliasesJSON).Scan(&memberID)

	if err != nil {
		return fmt.Errorf("failed to insert member: %w", err)
	}

	// Insert raw profile (Japanese)
	if um.RawProfile != nil {
		profileDataJSON, err := json.Marshal(um.RawProfile.DataEntries)
		if err != nil {
			return fmt.Errorf("failed to marshal raw profile data: %w", err)
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO member_profiles (member_id, language, display_name, catchphrase, description, profile_data)
			VALUES ($1, 'ja', $2, $3, $4, $5)
		`, memberID, nullString(um.RawProfile.EnglishName+" / "+um.RawProfile.JapaneseName),
			nullString(um.RawProfile.Catchphrase), nullString(um.RawProfile.Description), profileDataJSON)

		if err != nil {
			return fmt.Errorf("failed to insert raw profile: %w", err)
		}
	}

	// Insert Korean profile
	if um.KoProfile != nil {
		// Handle null highlights
		var highlightsJSON []byte
		if um.KoProfile.Highlights != nil {
			highlightsJSON, _ = json.Marshal(um.KoProfile.Highlights)
		} else {
			highlightsJSON = []byte("[]")
		}

		// Handle profile data
		var profileDataJSON []byte
		if um.KoProfile.Data != nil {
			profileDataJSON, _ = json.Marshal(um.KoProfile.Data)
		} else {
			profileDataJSON = []byte("[]")
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO member_profiles (member_id, language, display_name, catchphrase, summary, highlights, profile_data)
			VALUES ($1, 'ko', $2, $3, $4, $5, $6)
		`, memberID, nullString(um.KoProfile.DisplayName), nullString(um.KoProfile.Catchphrase),
			nullString(um.KoProfile.Summary), highlightsJSON, profileDataJSON)

		if err != nil {
			return fmt.Errorf("failed to insert Korean profile: %w", err)
		}
	}

	return nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

func printSummary(unified []*UnifiedMember) {
	log.Println("\n===== Migration Summary =====")
	log.Printf("Total members: %d", len(unified))

	active := 0
	graduated := 0
	ended := 0
	withChannel := 0
	withRawProfile := 0
	withKoProfile := 0

	for _, um := range unified {
		switch um.Status {
		case "active":
			active++
		case "graduated":
			graduated++
		case "ended":
			ended++
		}
		if um.ChannelID != "" {
			withChannel++
		}
		if um.RawProfile != nil {
			withRawProfile++
		}
		if um.KoProfile != nil {
			withKoProfile++
		}
	}

	log.Printf("Status: %d active, %d graduated, %d ended", active, graduated, ended)
	log.Printf("Data: %d with channel, %d with raw profile, %d with Korean profile",
		withChannel, withRawProfile, withKoProfile)
	log.Println("=============================")
}
