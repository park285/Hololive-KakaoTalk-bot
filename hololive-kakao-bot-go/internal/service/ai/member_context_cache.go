package ai

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"go.uber.org/zap"
	"google.golang.org/genai"
)

type MemberDirectoryBuilder struct {
	mu    sync.RWMutex
	cache string
}

func NewMemberDirectoryBuilder() *MemberDirectoryBuilder {
	return &MemberDirectoryBuilder{}
}

func (b *MemberDirectoryBuilder) MemberListWithAliases(members domain.MemberDataProvider) string {
	b.mu.RLock()
	if b.cache != "" {
		defer b.mu.RUnlock()
		return b.cache
	}
	b.mu.RUnlock()

	lines := make([]string, len(members.GetAllMembers()))
	for i, member := range members.GetAllMembers() {
		if member == nil {
			continue
		}
		lines[i] = strings.Join([]string{
			member.Name,
			strings.Join(member.GetAllAliases(), ","),
			member.ChannelID,
		}, "|")
	}

	joined := strings.Join(lines, "\n")

	b.mu.Lock()
	b.cache = joined
	b.mu.Unlock()

	return joined
}

func (b *MemberDirectoryBuilder) Invalidate() {
	b.mu.Lock()
	b.cache = ""
	b.mu.Unlock()
}

type MemberListCacheManager struct {
	modelManager *ModelManager
	logger       *zap.Logger

	mu        sync.RWMutex
	cached    string
	expiry    time.Time
	cancel    context.CancelFunc
	interval  time.Duration
	refreshMu sync.Mutex
}

func NewMemberListCacheManager(modelManager *ModelManager, logger *zap.Logger) *MemberListCacheManager {
	return &MemberListCacheManager{
		modelManager: modelManager,
		logger:       logger,
		interval:     23 * time.Hour,
	}
}

func (m *MemberListCacheManager) EnsureInitialized(ctx context.Context, members domain.MemberDataProvider) error {
	if members == nil || len(members.GetAllMembers()) == 0 {
		return fmt.Errorf("members data is empty")
	}

	m.mu.RLock()
	valid := m.cached != "" && time.Until(m.expiry) > 0
	m.mu.RUnlock()
	if valid {
		return nil
	}

	return m.refresh(ctx, members)
}

func (m *MemberListCacheManager) StartAutoRefresh(ctx context.Context, members domain.MemberDataProvider) {
	m.mu.Lock()
	if m.cancel != nil {
		m.mu.Unlock()
		return
	}
	refreshCtx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	m.mu.Unlock()

	go m.runAutoRefresh(refreshCtx, members)
}

func (m *MemberListCacheManager) Stop() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.mu.Unlock()
}

func (m *MemberListCacheManager) GetCachedName() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cached
}

func (m *MemberListCacheManager) Invalidate() {
	m.mu.Lock()
	m.cached = ""
	m.expiry = time.Time{}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.mu.Unlock()
}

func (m *MemberListCacheManager) refresh(ctx context.Context, members domain.MemberDataProvider) error {
	m.refreshMu.Lock()
	defer m.refreshMu.Unlock()

	client := m.modelManager.GetGeminiClient()
	if client == nil {
		return fmt.Errorf("gemini client not available")
	}

	memberNames := make([]string, 0, len(members.GetAllMembers()))
	for _, member := range members.GetAllMembers() {
		if member != nil && member.Name != "" {
			memberNames = append(memberNames, member.Name)
		}
	}
	if len(memberNames) == 0 {
		return fmt.Errorf("no valid member names")
	}

	memberListText := m.buildMemberListContent(memberNames)
	systemText := m.systemInstruction()

	config := &genai.CreateCachedContentConfig{
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
	}

	cachedContent, err := client.Caches.Create(ctx, m.modelManager.GetDefaultGeminiModel(), config)
	if err != nil {
		m.logger.Error("Failed to create member list cache", zap.Error(err))
		return err
	}

	m.mu.Lock()
	m.cached = cachedContent.Name
	m.expiry = time.Now().Add(24 * time.Hour)
	m.mu.Unlock()

	m.logger.Info("Member list cache created",
		zap.String("cache_name", cachedContent.Name),
		zap.Int("member_count", len(memberNames)),
		zap.Time("expiry", m.expiry),
	)

	return nil
}

func (m *MemberListCacheManager) runAutoRefresh(ctx context.Context, members domain.MemberDataProvider) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := m.refresh(ctx, members); err != nil {
				m.logger.Error("Failed to auto-refresh member list cache", zap.Error(err))
			} else {
				m.logger.Info("Member list cache auto-refreshed successfully")
			}
		case <-ctx.Done():
			m.logger.Info("Member list cache auto-refresh stopped")
			return
		}
	}
}

func (m *MemberListCacheManager) buildMemberListContent(memberNames []string) string {
	return fmt.Sprintf(`# Hololive Production Official Member Directory

##  Complete Member List (%d members)

%s

##  Purpose and Usage

This is the official roster of Hololive Production VTubers (English names).
Use this list to determine if user queries are Hololive-related.

Hololive is Japan's leading VTuber agency with talents from multiple generations and regions (Japan, Indonesia, English-speaking).
Each member has unique character design, personality, fanbase, and primarily streams on YouTube.

##  Classification Criteria

###  Hololive-Related (is_hololive_related=true)

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

###  Not Hololive-Related (is_hololive_related=false)

Mark as false when:

1. **General information requests** - Weather, time, news
2. **Completely unrelated topics** - Food, travel, general knowledge
3. **Questions about the bot itself** - "Who are you", "What can you do"

##  Intent Recognition - Ignore Surface Variations

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

##  Clarification Message Rules

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
		len(memberNames),
		strings.Join(memberNames, ", "),
	)
}

func (m *MemberListCacheManager) systemInstruction() string {
	return `You are a Hololive VTuber specialist assistant.
Your role: Analyze user questions to determine Hololive-relatedness and generate Korean clarification messages when needed.

Core responsibilities:
1. Identify core intent (ignore grammar, conjugations, formality)
2. Determine Hololive-relatedness (using member list above)
3. Generate polite Korean clarification messages when appropriate

Always understand user intent accurately and provide consistent responses.`
}
