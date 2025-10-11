package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kapu/hololive-kakao-bot-go/internal/config"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/prompt"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/ai"
)

type multiString []string

func (m *multiString) String() string {
	return strings.Join(*m, ",")
}

func (m *multiString) Set(value string) error {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		*m = append(*m, trimmed)
	}
	return nil
}

const (
	outputPath             = "internal/domain/data/official_profiles_ko.json"
	maxDataEntries         = 10
	requestDelay           = 750 * time.Millisecond
	defaultGeminiModel     = "gemini-2.5-pro"
	defaultMaxOutputTokens = 8196
	maxAttempts            = 6
	backoffBase            = 2 * time.Second
)

func main() {
	var force bool
	var slugCSV string
	var slugFlags multiString
	var useOpenAI bool
	var improveStyle bool
	var refineFromOriginal bool
	var generationModel string
	var maxOutputTokens int

	flag.BoolVar(&force, "force", false, "regenerate all translations even if cached")
	flag.StringVar(&slugCSV, "slugs", "", "comma-separated list of slugs to translate")
	flag.Var(&slugFlags, "slug", "slug to translate (can be specified multiple times)")
	flag.BoolVar(&useOpenAI, "use-openai", false, "enable OpenAI fallback for translation")
	flag.BoolVar(&improveStyle, "improve-style", false, "improve Korean writing style of existing translations")
	flag.BoolVar(&refineFromOriginal, "refine-from-original", false, "refine translations using original Japanese text")
	flag.StringVar(&generationModel, "model", defaultGeminiModel, "Gemini model to use (e.g., gemini-2.5-pro, gemini-2.5-flash)")
	flag.IntVar(&maxOutputTokens, "max-output-tokens", defaultMaxOutputTokens, "maximum output tokens override (set 0 to disable)")
	flag.Parse()

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	generationModel = strings.TrimSpace(generationModel)
	if generationModel == "" {
		generationModel = defaultGeminiModel
	}
	if maxOutputTokens < 0 {
		logger.Warn("max-output-tokens cannot be negative; override disabled")
		maxOutputTokens = 0
	}

	apiKey := strings.TrimSpace(os.Getenv("GEMINI_API_KEY"))
	openAIKey := strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))

	var cfg *config.Config
	var cfgErr error

	if apiKey == "" || (useOpenAI && openAIKey == "") {
		cfg, cfgErr = config.Load()
		if cfgErr != nil {
			logger.Fatal("failed to load config file", zap.Error(cfgErr))
			return
		}
	}

	if apiKey == "" && cfg != nil {
		apiKey = strings.TrimSpace(cfg.Gemini.APIKey)
	}
	if apiKey == "" {
		logger.Fatal("GEMINI_API_KEY is not configured in environment or config")
		return
	}

	if openAIKey == "" && cfg != nil {
		openAIKey = strings.TrimSpace(cfg.OpenAI.APIKey)
	}
	if useOpenAI && openAIKey == "" {
		logger.Warn("OPENAI_API_KEY not available; fallback disabled despite --use-openai flag")
		useOpenAI = false
	}

	ctx := context.Background()
	logger.Info("using Gemini configuration",
		zap.String("model", generationModel),
		zap.Int("max_output_tokens", maxOutputTokens),
	)

	mm, err := ai.NewModelManager(ctx, ai.ModelManagerConfig{
		GeminiAPIKey:       apiKey,
		DefaultGeminiModel: generationModel,
		OpenAIAPIKey:       openAIKey,
		EnableFallback:     useOpenAI && openAIKey != "",
	}, logger)
	if err != nil {
		logger.Fatal("failed to create model manager", zap.Error(err))
	}

	// Refine from original mode: refine translations using original Japanese text
	if refineFromOriginal {
		if err := runRefineFromOriginal(ctx, mm, slugFlags, slugCSV, generationModel, maxOutputTokens, logger); err != nil {
			logger.Fatal("refine from original failed", zap.Error(err))
		}
		return
	}

	// Style improvement mode: improve existing Korean translations
	if improveStyle {
		if err := runStyleImprovement(ctx, mm, slugFlags, slugCSV, logger); err != nil {
			logger.Fatal("style improvement failed", zap.Error(err))
		}
		return
	}

	// Original translation mode
	rawProfiles, err := domain.LoadProfiles()
	if err != nil {
		logger.Fatal("failed to load official profiles", zap.Error(err))
	}

	existingTranslations, err := domain.LoadTranslated()
	if err != nil {
		logger.Fatal("failed to load existing translations", zap.Error(err))
	}

	translations := make(map[string]*domain.Translated, len(rawProfiles))
	if !force {
		for slug, translated := range existingTranslations {
			if translated != nil {
				translations[slug] = translated
			}
		}
	}

	targetSlugs := make(map[string]struct{})
	for _, entry := range slugFlags {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			targetSlugs[trimmed] = struct{}{}
		}
	}
	if slugCSV != "" {
		for _, part := range strings.Split(slugCSV, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				targetSlugs[trimmed] = struct{}{}
			}
		}
	}

	slugs := make([]string, 0, len(rawProfiles))
	for slug := range rawProfiles {
		if len(targetSlugs) > 0 {
			if _, ok := targetSlugs[slug]; !ok {
				continue
			}
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		if len(targetSlugs) > 0 {
			logger.Warn("no matching slugs found", zap.Int("requested", len(targetSlugs)))
		} else {
			logger.Info("nothing to translate")
		}
		return
	}
	sort.Strings(slugs)

	logger.Info("starting translation run",
		zap.Int("total_raw", len(rawProfiles)),
		zap.Int("targets", len(slugs)),
		zap.Bool("force", force),
	)

	for _, slug := range slugs {
		profile := rawProfiles[slug]
		if profile == nil {
			continue
		}

		if !force {
			if _, ok := translations[slug]; ok {
				logger.Info("skip (cached)", zap.String("slug", slug), zap.String("member", profile.EnglishName))
				continue
			}
		}

		promptText, err := buildPrompt(profile)
		if err != nil {
			logger.Warn("failed to build prompt", zap.String("slug", slug), zap.Error(err))
			continue
		}

		translated, genErr := translateWithRetry(ctx, mm, promptText, profile, logger)
		if genErr != nil {
			logger.Error("translation failed", zap.String("slug", slug), zap.Error(genErr))
			continue
		}

		translations[slug] = translated

		logger.Info("translated", zap.String("slug", slug), zap.String("member", profile.EnglishName))
		time.Sleep(requestDelay)
	}

	if err := writeOutput(translations); err != nil {
		logger.Fatal("failed to write output", zap.Error(err))
	}

	logger.Info("translation completed", zap.Int("count", len(translations)), zap.String("output", outputPath))
}

func buildPrompt(profile *domain.TalentProfile) (string, error) {
	if profile == nil {
		return "", errors.New("profile is nil")
	}

	entries := make([]prompt.TranslateEntry, 0, len(profile.DataEntries))
	for _, entry := range profile.DataEntries {
		label := strings.TrimSpace(entry.Label)
		value := strings.TrimSpace(entry.Value)
		if label == "" || value == "" {
			continue
		}
		entries = append(entries, prompt.TranslateEntry{Label: label, Value: value})
	}

	if len(entries) > maxDataEntries {
		entries = entries[:maxDataEntries]
	}

	promptText, err := prompt.BuildTranslate(prompt.TranslateVars{
		EnglishName:    profile.EnglishName,
		JapaneseName:   profile.JapaneseName,
		Catchphrase:    profile.Catchphrase,
		Description:    profile.Description,
		DataEntries:    entries,
		MaxDataEntries: maxDataEntries,
	})
	if err != nil {
		return "", err
	}
	return promptText, nil
}

func translateWithRetry(ctx context.Context, mm *ai.ModelManager, promptText string, profile *domain.TalentProfile, logger *zap.Logger) (*domain.Translated, error) {
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var translated domain.Translated
		_, err := mm.GenerateJSON(ctx, promptText, ai.PresetBalanced, &translated, nil)
		if err == nil {
			normalizeTranslation(&translated, profile)
			return &translated, nil
		}

		lastErr = err
		if !isRecoverableError(err) || attempt == maxAttempts {
			break
		}

		sleep := backoffBase * time.Duration(attempt)
		errMsg := err.Error()
		if strings.Contains(errMsg, "Circuit OPEN") || strings.Contains(errMsg, "자동 복구 대기 중") {
			sleep = 35 * time.Second
		} else if strings.Contains(errMsg, "503") || strings.Contains(errMsg, "overloaded") {
			sleep = backoffBase * time.Duration(attempt+1)
		}

		logger.Warn("retrying translation",
			zap.String("member", profile.EnglishName),
			zap.Int("attempt", attempt),
			zap.Duration("sleep", sleep),
			zap.Error(err),
		)

		time.Sleep(sleep)
	}

	return nil, lastErr
}

func isRecoverableError(err error) bool {
	if err == nil {
		return false
	}

	msg := err.Error()
	recoverable := []string{
		"503",
		"UNAVAILABLE",
		"model is overloaded",
		"외부 AI 서비스 장애",
		"자동 복구 대기 중",
	}

	for _, key := range recoverable {
		if strings.Contains(msg, key) {
			return true
		}
	}

	return false
}

func normalizeTranslation(translated *domain.Translated, raw *domain.TalentProfile) {
	if translated == nil || raw == nil {
		return
	}

	if strings.TrimSpace(translated.DisplayName) == "" {
		if raw.JapaneseName != "" {
			translated.DisplayName = fmt.Sprintf("%s (%s)", raw.EnglishName, raw.JapaneseName)
		} else {
			translated.DisplayName = raw.EnglishName
		}
	}

	for i := range translated.Data {
		if i >= len(raw.DataEntries) {
			break
		}
		if shouldPreserveRawValue(raw.DataEntries[i].Label) {
			translated.Data[i].Value = raw.DataEntries[i].Value
		}
	}
}

func shouldPreserveRawValue(rawLabel string) bool {
	if rawLabel == "" {
		return false
	}

	keywords := []string{
		"ハッシュタグ",
		"ファンネーム",
		"推しマーク",
	}

	for _, kw := range keywords {
		if strings.Contains(rawLabel, kw) {
			return true
		}
	}

	return false
}

func writeOutput(data map[string]*domain.Translated) error {
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}

	serialized, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	tmpFile := outputPath + ".tmp"
	if err := os.WriteFile(tmpFile, serialized, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmpFile, outputPath); err != nil {
		return err
	}

	splitDir := filepath.Join(filepath.Dir(outputPath), "official_profiles_ko")
	if err := os.MkdirAll(splitDir, 0o755); err != nil {
		return err
	}

	for slug, profile := range data {
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

func runStyleImprovement(ctx context.Context, mm *ai.ModelManager, slugFlags multiString, slugCSV string, logger *zap.Logger) error {
	existingTranslations, err := domain.LoadTranslated()
	if err != nil {
		return fmt.Errorf("failed to load existing translations: %w", err)
	}

	targetSlugs := make(map[string]struct{})
	for _, entry := range slugFlags {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			targetSlugs[trimmed] = struct{}{}
		}
	}
	if slugCSV != "" {
		for _, part := range strings.Split(slugCSV, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				targetSlugs[trimmed] = struct{}{}
			}
		}
	}

	slugs := make([]string, 0, len(existingTranslations))
	for slug := range existingTranslations {
		if len(targetSlugs) > 0 {
			if _, ok := targetSlugs[slug]; !ok {
				continue
			}
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		logger.Info("no translations to improve")
		return nil
	}
	sort.Strings(slugs)

	logger.Info("starting style improvement",
		zap.Int("total", len(existingTranslations)),
		zap.Int("targets", len(slugs)),
	)

	improved := make(map[string]*domain.Translated, len(slugs))

	for _, slug := range slugs {
		profile := existingTranslations[slug]
		if profile == nil {
			continue
		}

		currentJSON, err := json.MarshalIndent(profile, "", "  ")
		if err != nil {
			logger.Warn("failed to marshal current translation", zap.String("slug", slug), zap.Error(err))
			continue
		}

		promptText, err := prompt.BuildStyle(prompt.StyleVars{
			CurrentTranslation: string(currentJSON),
		})
		if err != nil {
			logger.Warn("failed to build improvement prompt", zap.String("slug", slug), zap.Error(err))
			continue
		}

		var improvedProfile domain.Translated
		_, genErr := mm.GenerateJSON(ctx, promptText, ai.PresetBalanced, &improvedProfile, nil)
		if genErr != nil {
			logger.Error("style improvement failed", zap.String("slug", slug), zap.Error(genErr))
			continue
		}

		// Preserve data array from original
		if len(profile.Data) > 0 {
			improvedProfile.Data = profile.Data
		}

		improved[slug] = &improvedProfile
		logger.Info("improved", zap.String("slug", slug))
		time.Sleep(requestDelay)
	}

	// Merge improved profiles with existing ones
	for slug, profile := range existingTranslations {
		if _, wasImproved := improved[slug]; !wasImproved {
			improved[slug] = profile
		}
	}

	if err := writeOutput(improved); err != nil {
		return fmt.Errorf("failed to write improved translations: %w", err)
	}

	logger.Info("style improvement completed", zap.Int("improved_count", len(slugs)))
	return nil
}

func runRefineFromOriginal(ctx context.Context, mm *ai.ModelManager, slugFlags multiString, slugCSV string, model string, maxOutputTokens int, logger *zap.Logger) error {
	rawProfiles, err := domain.LoadProfiles()
	if err != nil {
		return fmt.Errorf("failed to load original profiles: %w", err)
	}

	existingTranslations, err := domain.LoadTranslated()
	if err != nil {
		return fmt.Errorf("failed to load existing translations: %w", err)
	}

	targetSlugs := make(map[string]struct{})
	for _, entry := range slugFlags {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			targetSlugs[trimmed] = struct{}{}
		}
	}
	if slugCSV != "" {
		for _, part := range strings.Split(slugCSV, ",") {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				targetSlugs[trimmed] = struct{}{}
			}
		}
	}

	slugs := make([]string, 0, len(existingTranslations))
	for slug := range existingTranslations {
		if len(targetSlugs) > 0 {
			if _, ok := targetSlugs[slug]; !ok {
				continue
			}
		}
		slugs = append(slugs, slug)
	}
	if len(slugs) == 0 {
		logger.Info("no translations to refine")
		return nil
	}
	sort.Strings(slugs)

	logger.Info("starting refinement from original",
		zap.Int("total", len(existingTranslations)),
		zap.Int("targets", len(slugs)),
		zap.String("model", model),
		zap.Int("max_output_tokens", maxOutputTokens),
	)

	refined := make(map[string]*domain.Translated, len(slugs))

	for _, slug := range slugs {
		original := rawProfiles[slug]
		current := existingTranslations[slug]
		if original == nil || current == nil {
			continue
		}

		currentJSON, err := json.MarshalIndent(current, "", "  ")
		if err != nil {
			logger.Warn("failed to marshal current translation", zap.String("slug", slug), zap.Error(err))
			continue
		}

		promptText, err := prompt.BuildRefine(prompt.RefineVars{
			OriginalCatchphrase: original.Catchphrase,
			OriginalDescription: original.Description,
			CurrentTranslation:  string(currentJSON),
		})
		if err != nil {
			logger.Warn("failed to build refinement prompt", zap.String("slug", slug), zap.Error(err))
			continue
		}

		// Use configured Gemini model with optional max token override
		opts := &ai.GenerateOptions{
			Model: model,
		}
		if maxOutputTokens > 0 {
			opts.Overrides = &ai.ModelConfig{
				MaxOutputTokens: maxOutputTokens,
			}
		}

		var refinedProfile domain.Translated
		_, genErr := mm.GenerateJSON(ctx, promptText, ai.PresetBalanced, &refinedProfile, opts)
		if genErr != nil {
			logger.Error("refinement failed", zap.String("slug", slug), zap.Error(genErr))
			continue
		}

		// Preserve data array from original
		if len(current.Data) > 0 {
			refinedProfile.Data = current.Data
		}

		refined[slug] = &refinedProfile
		logger.Info("refined", zap.String("slug", slug))
		time.Sleep(requestDelay)
	}

	// Merge refined profiles with existing ones
	for slug, profile := range existingTranslations {
		if _, wasRefined := refined[slug]; !wasRefined {
			refined[slug] = profile
		}
	}

	if err := writeOutput(refined); err != nil {
		return fmt.Errorf("failed to write refined translations: %w", err)
	}

	logger.Info("refinement from original completed", zap.Int("refined_count", len(slugs)))
	return nil
}
