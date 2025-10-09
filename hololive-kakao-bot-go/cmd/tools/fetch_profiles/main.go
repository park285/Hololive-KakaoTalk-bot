package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"go.uber.org/zap"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
)

const (
	baseURL        = "https://hololive.hololivepro.com/talents"
	userAgent      = "Mozilla/5.0 (compatible; HololiveKakaoBot/1.0; +https://hololive.hololivepro.com)"
	acceptLanguage = "ja,en;q=0.8,ko;q=0.6"
	requestTimeout = 15 * time.Second
	delayBetween   = 350 * time.Millisecond
	outputFile     = "internal/domain/data/official_profiles_raw.json"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx := context.Background()

	talents, err := domain.LoadOfficialTalents()
	if err != nil {
		logger.Fatal("failed to load official talents list", zap.Error(err))
	}

	httpClient := &http.Client{Timeout: requestTimeout}

	profiles := make(map[string]*domain.TalentProfile, len(talents.Talents))
	for idx, talent := range talents.Talents {
		if talent == nil || strings.TrimSpace(talent.English) == "" {
			continue
		}

		slug := talent.Slug()
		english := strings.TrimSpace(talent.English)

		profileURL := fmt.Sprintf("%s/%s/", baseURL, slug)
		logger.Info("Fetching profile", zap.Int("index", idx+1), zap.String("slug", slug), zap.String("url", profileURL))

		profile, err := fetchProfile(ctx, httpClient, profileURL, english, slug)
		if err != nil {
			logger.Error("failed to fetch profile", zap.String("slug", slug), zap.Error(err))
			continue
		}

		profiles[slug] = profile
		time.Sleep(delayBetween)
	}

	if len(profiles) == 0 {
		logger.Fatal("no profiles fetched")
	}

	if err := writeProfiles(profiles); err != nil {
		logger.Fatal("failed to write profiles", zap.Error(err))
	}

	logger.Info("Profile fetch completed", zap.Int("count", len(profiles)), zap.String("output", outputFile))
}

func fetchProfile(ctx context.Context, client *http.Client, url, englishName, slug string) (*domain.TalentProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept-Language", acceptLanguage)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}

	profile := &domain.TalentProfile{
		Slug:        slug,
		OfficialURL: url,
	}

	rightBox := doc.Find(".right_box").First()
	if rightBox.Length() == 0 {
		return nil, fmt.Errorf("profile container not found")
	}

	header := rightBox.Find("h1").First()
	if header.Length() > 0 {
		japanese := strings.TrimSpace(header.Clone().Children().Remove().Text())
		english := strings.TrimSpace(header.Find("span").First().Text())

		if english != "" {
			profile.EnglishName = english
		} else {
			profile.EnglishName = englishName
		}

		if japanese != "" {
			profile.JapaneseName = japanese
		}
	} else {
		profile.EnglishName = englishName
	}

	catchText := normalizeText(rightBox.Find("p.catch").First().Text())
	profile.Catchphrase = catchText

	descText := normalizeText(rightBox.Find("p.txt").First().Text())
	profile.Description = descText

	profile.SocialLinks = extractSocialLinks(rightBox.Find(".t_sns a"))
	profile.DataEntries = extractDataEntries(doc.Find(".talent_data .table_box dl"))

	return profile, nil
}

func extractSocialLinks(selection *goquery.Selection) []domain.TalentSocialLink {
	links := make([]domain.TalentSocialLink, 0, selection.Length())
	selection.Each(func(_ int, sel *goquery.Selection) {
		label := strings.TrimSpace(sel.Text())
		href, _ := sel.Attr("href")
		url := strings.TrimSpace(href)
		if label == "" || url == "" {
			return
		}
		links = append(links, domain.TalentSocialLink{Label: label, URL: url})
	})
	return links
}

func extractDataEntries(selection *goquery.Selection) []domain.TalentProfileEntry {
	entries := make([]domain.TalentProfileEntry, 0, selection.Length())
	selection.Each(func(_ int, sel *goquery.Selection) {
		label := strings.TrimSpace(sel.Find("dt").First().Text())
		value := normalizeText(sel.Find("dd").First().Text())
		if label == "" || value == "" {
			return
		}
		entries = append(entries, domain.TalentProfileEntry{Label: label, Value: value})
	})
	return entries
}

func normalizeText(input string) string {
	input = strings.ReplaceAll(input, "\u00a0", " ")
	lines := strings.Split(input, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		filtered = append(filtered, line)
	}
	return strings.Join(filtered, "\n")
}

func writeProfiles(profiles map[string]*domain.TalentProfile) error {
	if err := os.MkdirAll(filepath.Dir(outputFile), 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := outputFile + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, outputFile); err != nil {
		return err
	}

	splitDir := filepath.Join(filepath.Dir(outputFile), "official_profiles_raw")
	if err := os.MkdirAll(splitDir, 0o755); err != nil {
		return err
	}

	for slug, profile := range profiles {
		bytes, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal profile %s: %w", slug, err)
		}

		tmp := filepath.Join(splitDir, slug+".json.tmp")
		target := filepath.Join(splitDir, slug+".json")
		if err := os.WriteFile(tmp, bytes, 0o644); err != nil {
			return fmt.Errorf("failed to write profile %s: %w", slug, err)
		}
		if err := os.Rename(tmp, target); err != nil {
			return fmt.Errorf("failed to finalize profile %s: %w", slug, err)
		}
	}

	return nil
}
