package service

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/constants"
	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/prompt"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

// Control characters pattern (compiled once at package init)
var controlCharsPattern = regexp.MustCompile(`[\x00-\x1F\x7F]`)

// Jjang suffix pattern (compiled once)
var jjangSuffixPattern = regexp.MustCompile(`([가-힣]+)짱`)

// Whitespace pattern (compiled once)
var whitespacePattern = regexp.MustCompile(`\s+`)

// ParseCacheEntry holds cached parse results
type ParseCacheEntry struct {
	Result    *domain.ParseResults
	Metadata  *GenerateMetadata
	Timestamp time.Time
}

// GeminiService handles natural language processing with Gemini
type GeminiService struct {
	modelManager           *ModelManager
	logger                 *zap.Logger
	memberListCache        string
	memberListCachedName   string          // Gemini CachedContent name for member list
	memberListCacheMu      sync.RWMutex
	memberListCacheExpiry  time.Time
	parseCache             map[string]*ParseCacheEntry
	parseCacheMu           sync.RWMutex
	parseCacheTTL          time.Duration
}

// NewGeminiService creates a new Gemini service
func NewGeminiService(modelManager *ModelManager, logger *zap.Logger) *GeminiService {
	return &GeminiService{
		modelManager:  modelManager,
		logger:        logger,
		parseCache:    make(map[string]*ParseCacheEntry),
		parseCacheTTL: 5 * time.Minute,
	}
}

// InitializeMemberListCache initializes the context cache for member list
func (gs *GeminiService) InitializeMemberListCache(ctx context.Context, membersData *domain.MembersData) error {
	if membersData == nil || len(membersData.Members) == 0 {
		return fmt.Errorf("members data is empty")
	}

	client := gs.modelManager.GetGeminiClient()
	if client == nil {
		return fmt.Errorf("gemini client not available")
	}

	// Extract member names
	memberNames := make([]string, 0, len(membersData.Members))
	for _, member := range membersData.Members {
		if member != nil && member.Name != "" {
			memberNames = append(memberNames, member.Name)
		}
	}

	if len(memberNames) == 0 {
		return fmt.Errorf("no valid member names")
	}

	// Create cached content with member list (must be 1024+ tokens for Gemini Flash)
	// Using English for token efficiency (Korean uses 2-3x more tokens)
	memberListText := fmt.Sprintf(`# Hololive Production Official Member Directory

## 📋 Complete Member List (%d members)

%s

## 🎯 Purpose and Usage

This is the official roster of Hololive Production VTubers (English names).
Use this list to determine if user queries are Hololive-related.

Hololive is Japan's leading VTuber agency with talents from multiple generations and regions (Japan, Indonesia, English-speaking).
Each member has unique character design, personality, fanbase, and primarily streams on YouTube.

## 🔍 Classification Criteria

### ✅ Hololive-Related (is_hololive_related=true)

Mark as true when:

1. **Questions about members in the list above**
   - Profile, information, introduction requests
   - Activities, stream schedules
   - Characteristics, personality, content

2. **Questions mentioning people NOT in the list**
   - Example: "Tell me about Hatsune Miku" → Not Hololive, but still true (provide clarification)
   - Example: "Introduce Kizuna AI" → Not Hololive, but still true (provide clarification)
   - Provide clarification message that they're not Hololive members

3. **Questions about Hololive content, schedules, events**
   - Stream schedules, collaborations, events

### ❌ Not Hololive-Related (is_hololive_related=false)

Mark as false when:

1. **General information requests** - Weather, time, news
2. **Completely unrelated topics** - Food, travel, general knowledge
3. **Questions about the bot itself** - "Who are you", "What can you do"

## 💬 Intent Recognition - Ignore Surface Variations

Users express identical intents in vastly different ways.
**Focus ONLY on core intent, ignore all surface variations.**

### Critical: These are IDENTICAL intents

**Information Request Intent:**
All Korean verb conjugations of "to tell/inform" are the SAME:
- "알려줘" = "알려줄래?" = "알려주세요" = "알려줄까?" = "알려드릴까요?"
- "소개해" = "소개해줘" = "소개해줄래?" = "소개 좀"
- "말해" = "말해줘" = "말해줄래?" = "말해봐"

**Identity Question Intent:**
- "누구야" = "누구예요" = "누구입니까" = "누군지" = "누구세요"

### Ignore These Differences

1. **Formality levels** - Formal (주세요) vs casual (줘) = SAME INTENT
2. **Sentence types** - Question (줄래?) vs command (줘) vs statement (준다) = SAME INTENT
3. **All verb conjugations** - Treat all forms of a verb as identical
4. **Grammar variations** - Particles, word order, spacing

## 📝 Clarification Message Rules

When person mentioned is NOT in the member list:

**Settings:**
- is_hololive_related=true (it's a person information request)

**Message format (MUST be in Korean):**
"누구를 말씀하신 건지 잘 모르겠어요. \"<person-name>\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요."

**Candidate rules:**
- Fill only if 80%%+ confident about which Hololive member
- Leave empty if uncertain or not in list

**Language:**
- Response message MUST be in Korean (never English)
- Use polite and natural Korean expressions

Apply these rules consistently for accurate classification and responses.`,
		len(memberNames), strings.Join(memberNames, ", "))
	
	systemText := `You are a Hololive VTuber specialist assistant.
Your role: Analyze user questions to determine Hololive-relatedness and generate Korean clarification messages when needed.

Core responsibilities:
1. Identify core intent (ignore grammar, conjugations, formality)
2. Determine Hololive-relatedness (using member list above)
3. Generate polite Korean clarification messages when appropriate

Always understand user intent accurately and provide consistent responses.`
	
	cachedContent, err := client.Caches.Create(ctx, gs.modelManager.GetDefaultGeminiModel(), &genai.CreateCachedContentConfig{
		Contents: []*genai.Content{
			{
				Role: "user",
				Parts: []*genai.Part{
					{Text: memberListText},
				},
			},
		},
		SystemInstruction: &genai.Content{
			Parts: []*genai.Part{
				{Text: systemText},
			},
		},
		TTL: 24 * time.Hour,
	})

	if err != nil {
		gs.logger.Error("Failed to create member list cache", zap.Error(err))
		return err
	}

	gs.memberListCacheMu.Lock()
	gs.memberListCachedName = cachedContent.Name
	gs.memberListCacheExpiry = time.Now().Add(24 * time.Hour)
	gs.memberListCacheMu.Unlock()

	gs.logger.Info("Member list cache created",
		zap.String("cache_name", cachedContent.Name),
		zap.Int("member_count", len(memberNames)),
		zap.Time("expiry", gs.memberListCacheExpiry),
	)

	// Start auto-refresh goroutine
	go gs.autoRefreshMemberListCache(ctx, membersData)

	return nil
}

// autoRefreshMemberListCache automatically refreshes the cache every 24 hours
func (gs *GeminiService) autoRefreshMemberListCache(ctx context.Context, membersData *domain.MembersData) {
	ticker := time.NewTicker(23 * time.Hour) // Refresh 1 hour before expiry
	defer ticker.Stop()

	for range ticker.C {
		gs.logger.Info("Auto-refreshing member list cache")
		
		if err := gs.InitializeMemberListCache(ctx, membersData); err != nil {
			gs.logger.Error("Failed to auto-refresh member list cache", zap.Error(err))
		} else {
			gs.logger.Info("Member list cache auto-refreshed successfully")
		}
	}
}

// GetMemberListCacheName returns the current cached content name
func (gs *GeminiService) GetMemberListCacheName() string {
	gs.memberListCacheMu.RLock()
	defer gs.memberListCacheMu.RUnlock()
	return gs.memberListCachedName
}

// ParseNaturalLanguage parses natural language query into command(s)
func (gs *GeminiService) ParseNaturalLanguage(ctx context.Context, query string, membersData *domain.MembersData) (*domain.ParseResults, *GenerateMetadata, error) {
	normalizedQuery := strings.ToLower(strings.TrimSpace(query))
	cacheKey := fmt.Sprintf("parse:%s", normalizedQuery)

	// Check cache
	gs.parseCacheMu.RLock()
	cached, found := gs.parseCache[cacheKey]
	gs.parseCacheMu.RUnlock()

	if found {
		age := time.Since(cached.Timestamp)
		if age < gs.parseCacheTTL {
			gs.logger.Debug("Parse cache hit",
				zap.String("query", query),
				zap.Duration("age", age),
			)
			return cached.Result, cached.Metadata, nil
		}

		// Expired - remove
		gs.parseCacheMu.Lock()
		delete(gs.parseCache, cacheKey)
		gs.parseCacheMu.Unlock()
	}

	// Parse fresh
	gs.logger.Debug("Parse cache miss", zap.String("query", query))
	result, metadata, err := gs.parseNaturalLanguageImpl(ctx, query, membersData)
	if err != nil {
		return nil, nil, err
	}

	// Cache result
	gs.parseCacheMu.Lock()
	gs.parseCache[cacheKey] = &ParseCacheEntry{
		Result:    result,
		Metadata:  metadata,
		Timestamp: time.Now(),
	}
	gs.parseCacheMu.Unlock()

	// Auto cleanup
	go func() {
		time.Sleep(gs.parseCacheTTL)
		gs.parseCacheMu.Lock()
		delete(gs.parseCache, cacheKey)
		gs.parseCacheMu.Unlock()
		gs.logger.Debug("Parse cache expired", zap.String("query", query))
	}()

	return result, metadata, nil
}

// parseNaturalLanguageImpl implements the actual parsing logic
func (gs *GeminiService) parseNaturalLanguageImpl(ctx context.Context, query string, membersData *domain.MembersData) (*domain.ParseResults, *GenerateMetadata, error) {
	sanitized := gs.sanitizeInput(query)
	if sanitized == "" {
		gs.logger.Warn("Empty query after sanitization")
		return &domain.ParseResults{
				Single: &domain.ParseResult{
					Command:    domain.CommandUnknown,
					Params:     make(map[string]any),
					Confidence: 0,
					Reasoning:  "입력된 질문이 비어 있습니다.",
				},
			}, &GenerateMetadata{
				Provider: "None",
				Model:    "n/a",
			}, nil
	}

	// Normalize "짱" suffix
	normalized := jjangSuffixPattern.ReplaceAllString(sanitized, "$1")
	if normalized != sanitized {
		gs.logger.Info("Normalized query suffix",
			zap.String("original", sanitized),
			zap.String("normalized", normalized),
		)
	}

	// Build prompt
	promptText := gs.buildPrompt(normalized, membersData)

	// Call AI with precise preset for accurate command parsing
	var rawResult any
	metadata, err := gs.modelManager.GenerateJSON(ctx, promptText, PresetPrecise, &rawResult, nil)
	if err != nil {
		// Check if circuit open error
		if strings.Contains(err.Error(), "외부 AI 서비스 장애") {
			return nil, nil, err
		}

		gs.logger.Error("Failed to parse natural language", zap.Error(err))
		return &domain.ParseResults{
				Single: &domain.ParseResult{
					Command:    domain.CommandUnknown,
					Params:     make(map[string]any),
					Confidence: 0,
					Reasoning:  "자연어 처리 중 오류가 발생했습니다.",
				},
			}, &GenerateMetadata{
				Provider: "Unknown",
				Model:    "error",
			}, nil
	}

	// Parse result - can be single object or array
	parseResults, err := gs.parseAIResponse(rawResult)
	if err != nil {
		gs.logger.Error("Failed to parse AI response", zap.Error(err))
		return &domain.ParseResults{
			Single: &domain.ParseResult{
				Command:    domain.CommandUnknown,
				Params:     make(map[string]any),
				Confidence: 0,
				Reasoning:  "AI 응답 형식이 올바르지 않습니다.",
			},
		}, metadata, nil
	}

	return parseResults, metadata, nil
}

// parseAIResponse parses the AI response into ParseResults
func (gs *GeminiService) parseAIResponse(rawResult any) (*domain.ParseResults, error) {
	// Check if array (multiple commands)
	if arr, ok := rawResult.([]any); ok {
		results := make([]*domain.ParseResult, 0, len(arr))
		for _, item := range arr {
			pr, err := gs.parseResultObject(item)
			if err != nil {
				gs.logger.Warn("Failed to parse array item", zap.Error(err))
				continue
			}
			results = append(results, pr)
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("no valid results in array")
		}
		return &domain.ParseResults{Multiple: results}, nil
	}

	// Single object
	pr, err := gs.parseResultObject(rawResult)
	if err != nil {
		return nil, err
	}
	return &domain.ParseResults{Single: pr}, nil
}

// parseResultObject parses a single result object
func (gs *GeminiService) parseResultObject(obj any) (*domain.ParseResult, error) {
	m, ok := obj.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected object, got %T", obj)
	}

	pr := &domain.ParseResult{
		Params: make(map[string]any),
	}

	// Parse command
	if cmd, ok := m["command"].(string); ok {
		pr.Command = domain.CommandType(strings.ToLower(cmd))
	} else {
		return nil, fmt.Errorf("missing command field")
	}

	// Parse params
	if params, ok := m["params"].(map[string]any); ok {
		pr.Params = params
	}

	// Parse confidence
	if conf, ok := m["confidence"].(float64); ok {
		pr.Confidence = conf
	}

	// Parse reasoning
	if reasoning, ok := m["reasoning"].(string); ok {
		pr.Reasoning = reasoning
	}

	return pr, nil
}

// SelectBestChannel selects the best matching channel from candidates
func (gs *GeminiService) SelectBestChannel(ctx context.Context, query string, candidates []*domain.Channel) (*domain.Channel, error) {
	if len(candidates) == 0 {
		return nil, nil
	}

	if len(candidates) == 1 {
		return candidates[0], nil
	}

	sanitized := gs.sanitizeInput(query)
	if sanitized == "" {
		gs.logger.Warn("Channel selection skipped: empty query")
		return nil, nil
	}

	// Build prompt
	promptText := prompt.BuildSelectorPrompt(sanitized, candidates)

	// Call AI
	var selection domain.ChannelSelection
	metadata, err := gs.modelManager.GenerateJSON(ctx, promptText, PresetPrecise, &selection, &GenerateOptions{
		Overrides: &ModelConfig{MaxOutputTokens: 256},
	})

	if err != nil {
		gs.logger.Error("Failed to select channel", zap.Error(err))
		return nil, err
	}

	gs.logger.Info("Channel selection",
		zap.String("query", query),
		zap.Int("selected_index", selection.SelectedIndex),
		zap.Float64("confidence", selection.Confidence),
		zap.String("reasoning", selection.Reasoning),
		zap.String("provider", metadata.Provider),
	)

	// Check confidence
	if selection.SelectedIndex == -1 || selection.Confidence < 0.7 {
		gs.logger.Warn("Low confidence in channel selection")
		return nil, nil
	}

	if selection.SelectedIndex < 0 || selection.SelectedIndex >= len(candidates) {
		gs.logger.Error("Invalid selected index",
			zap.Int("index", selection.SelectedIndex),
			zap.Int("candidates", len(candidates)),
		)
		return nil, nil
	}

	return candidates[selection.SelectedIndex], nil
}

// GenerateClarificationMessage produces a user-facing clarification message when member matching fails
func (gs *GeminiService) GenerateClarificationMessage(ctx context.Context, query string) (string, *GenerateMetadata, error) {
	sanitized := gs.sanitizeInput(query)
	if sanitized == "" {
		sanitized = strings.TrimSpace(query)
	}

	if sanitized == "" {
		return "", nil, fmt.Errorf("empty query for clarification")
	}

	promptText := prompt.BuildClarificationPrompt(sanitized)

	var clarification domain.ClarificationResponse
	metadata, err := gs.modelManager.GenerateJSON(ctx, promptText, PresetBalanced, &clarification, &GenerateOptions{
		Overrides: &ModelConfig{MaxOutputTokens: 256},
	})
	if err != nil {
		gs.logger.Error("Failed to generate clarification message", zap.Error(err))
		return "", nil, err
	}

	candidate := strings.TrimSpace(clarification.Candidate)
	message := strings.TrimSpace(clarification.Message)

	if candidate == "" {
		candidate = extractQuotedCandidate(message)
	}
	if candidate == "" {
		candidate = sanitized
	}

	finalMessage := buildClarificationSentence(candidate)

	if metadata != nil {
		gs.logger.Info("Clarification message generated",
			zap.String("provider", metadata.Provider),
			zap.String("model", metadata.Model),
			zap.Bool("used_fallback", metadata.UsedFallback),
			zap.String("candidate", candidate),
		)
	}

	return finalMessage, metadata, nil
}

// GenerateSmartClarification produces a Hololive-aware clarification message
func (gs *GeminiService) GenerateSmartClarification(ctx context.Context, query string, membersData *domain.MembersData) (*domain.SmartClarificationResponse, *GenerateMetadata, error) {
	sanitized := gs.sanitizeInput(query)
	if sanitized == "" {
		sanitized = strings.TrimSpace(query)
	}

	if sanitized == "" {
		return nil, nil, fmt.Errorf("empty query for smart clarification")
	}

	// Check if we have cached member list
	cachedName := gs.GetMemberListCacheName()
	var promptText string
	var generateOpts *GenerateOptions

	if cachedName != "" {
		// Use cached member list
		promptText = prompt.BuildSmartClarificationPromptWithoutMembers(sanitized)
		generateOpts = &GenerateOptions{
			Overrides:     &ModelConfig{MaxOutputTokens: 512},
			CachedContent: cachedName,
		}
		gs.logger.Debug("Using cached member list", zap.String("cache_name", cachedName))
	} else {
		// Fallback: embed member list in prompt
		var memberNames []string
		if membersData != nil && len(membersData.Members) > 0 {
			memberNames = make([]string, 0, len(membersData.Members))
			for _, member := range membersData.Members {
				if member != nil && member.Name != "" {
					memberNames = append(memberNames, member.Name)
				}
			}
		}
		promptText = prompt.BuildSmartClarificationPrompt(sanitized, memberNames)
		generateOpts = &GenerateOptions{
			Overrides: &ModelConfig{MaxOutputTokens: 512},
		}
		gs.logger.Warn("Member list cache not available, embedding in prompt")
	}

	var response domain.SmartClarificationResponse
	metadata, err := gs.modelManager.GenerateJSON(ctx, promptText, PresetBalanced, &response, generateOpts)
	if err != nil {
		gs.logger.Error("Failed to generate smart clarification", zap.Error(err))
		return nil, nil, err
	}

	// Ensure message is set if hololive-related
	if response.IsHololiveRelated && strings.TrimSpace(response.Message) == "" {
		candidate := strings.TrimSpace(response.Candidate)
		if candidate == "" {
			candidate = sanitized
		}
		response.Message = buildClarificationSentence(candidate)
	}

	if metadata != nil {
		gs.logger.Info("Smart clarification generated",
			zap.String("provider", metadata.Provider),
			zap.String("model", metadata.Model),
			zap.Bool("used_fallback", metadata.UsedFallback),
			zap.Bool("is_hololive_related", response.IsHololiveRelated),
			zap.String("candidate", response.Candidate),
			zap.Bool("used_cache", cachedName != ""),
		)
	}

	return &response, metadata, nil
}

func buildClarificationSentence(rawCandidate string) string {
	candidate := strings.TrimSpace(rawCandidate)
	if candidate == "" {
		candidate = "요청"
	}
	escaped := strings.ReplaceAll(candidate, `"`, "'")
	return fmt.Sprintf("누구를 말씀하신 건지 잘 모르겠어요. \"%s\"를 말씀하신 건가요? 홀로라이브 소속이 맞는지 확인하신 뒤 다시 질문해 주세요.", escaped)
}

func extractQuotedCandidate(message string) string {
	if message == "" {
		return ""
	}
	start := strings.Index(message, "\"")
	if start == -1 {
		return ""
	}
	remaining := message[start+1:]
	end := strings.Index(remaining, "\"")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(remaining[:end])
}

// buildPrompt builds the parser prompt with member list
func (gs *GeminiService) buildPrompt(query string, membersData *domain.MembersData) string {
	if gs.memberListCache == "" {
		gs.memberListCache = gs.buildMemberListWithAliases(membersData)
		gs.logger.Debug("Member list cached",
			zap.Int("members", len(membersData.Members)),
			zap.Int("size", len(gs.memberListCache)),
		)
	}

	return prompt.BuildParserPrompt(prompt.ParserPromptVars{
		MemberCount:       len(membersData.Members),
		MemberListWithIDs: gs.memberListCache,
		UserQuery:         query,
	})
}

// buildMemberListWithAliases builds formatted member list with aliases
func (gs *GeminiService) buildMemberListWithAliases(membersData *domain.MembersData) string {
	lines := make([]string, len(membersData.Members))

	for i, member := range membersData.Members {
		allAliases := member.GetAllAliases()
		aliasesStr := strings.Join(allAliases, ",")

		// Format: "EnglishName|aliases|channelID"
		lines[i] = fmt.Sprintf("%s|%s|%s", member.Name, aliasesStr, member.ChannelID)
	}

	return strings.Join(lines, "\n")
}

// sanitizeInput sanitizes user input
func (gs *GeminiService) sanitizeInput(input string) string {
	// Remove control characters
	withoutControl := controlCharsPattern.ReplaceAllString(input, " ")

	// Normalize whitespace
	normalized := whitespacePattern.ReplaceAllString(withoutControl, " ")
	trimmed := strings.TrimSpace(normalized)

	if trimmed == "" {
		return ""
	}

	// Limit length
	if len(trimmed) > constants.AIInputLimits.MaxQueryLength {
		return trimmed[:constants.AIInputLimits.MaxQueryLength]
	}

	return trimmed
}

// InvalidateMemberCache invalidates the member list cache
func (gs *GeminiService) InvalidateMemberCache() {
	gs.memberListCache = ""
	gs.logger.Info("Member list cache invalidated")
}

// ClassifyMemberInfoIntent classifies whether a query is asking for member information
func (gs *GeminiService) ClassifyMemberInfoIntent(ctx context.Context, query string) (*domain.MemberIntentClassification, *GenerateMetadata, error) {
	// Simple implementation - can be enhanced later
	sanitized := gs.sanitizeInput(query)
	if sanitized == "" {
		return &domain.MemberIntentClassification{
			Intent:     domain.MemberIntentOther,
			Confidence: 1.0,
			Reasoning:  "Empty query",
		}, &GenerateMetadata{Provider: "None", Model: "n/a"}, nil
	}

	// Basic keyword matching for now
	memberKeywords := []string{"누구", "알려", "소개", "프로필", "정보", "멤버"}
	lowerQuery := strings.ToLower(sanitized)

	for _, keyword := range memberKeywords {
		if strings.Contains(lowerQuery, keyword) {
			return &domain.MemberIntentClassification{
				Intent:     domain.MemberIntentMemberInfo,
				Confidence: 0.8,
				Reasoning:  fmt.Sprintf("Contains keyword: %s", keyword),
			}, &GenerateMetadata{Provider: "Rule", Model: "keyword-match"}, nil
		}
	}

	return &domain.MemberIntentClassification{
		Intent:     domain.MemberIntentOther,
		Confidence: 0.6,
		Reasoning:  "No member info keywords detected",
	}, &GenerateMetadata{Provider: "Rule", Model: "keyword-match"}, nil
}
