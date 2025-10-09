package service

import (
	"context"
	"fmt"
	"strings"

	"github.com/kapu/hololive-kakao-bot-go/internal/domain"
	"github.com/kapu/hololive-kakao-bot-go/internal/prompt"
	"github.com/kapu/hololive-kakao-bot-go/internal/util"
	"go.uber.org/zap"
)

const (
	translationLocale         = "ko"
	cacheKeyProfileTranslated = "hololive:profile:translated:%s:%s"
	maxPromptDataEntries      = 10
)

// OfficialProfileService serves pre-fetched official talent profiles and manages translations.
type OfficialProfileService struct {
	cache         *CacheService
	modelManager  *ModelManager
	logger        *zap.Logger
	membersData   *domain.MembersData
	profiles      map[string]*domain.TalentProfile // slug -> profile
	translations  map[string]*domain.TranslatedTalentProfile
	englishToSlug map[string]string
	channelToSlug map[string]string
}

// NewOfficialProfileService creates a new OfficialProfileService from embedded datasets.
func NewOfficialProfileService(cache *CacheService, membersData *domain.MembersData, modelManager *ModelManager, logger *zap.Logger) (*OfficialProfileService, error) {
	if membersData == nil {
		return nil, fmt.Errorf("members data is nil")
	}
	if logger == nil {
		logger = zap.NewNop()
	}

	profiles, err := domain.LoadOfficialProfiles()
	if err != nil {
		return nil, fmt.Errorf("failed to load official profiles dataset: %w", err)
	}

	preTranslated, err := domain.LoadOfficialTranslatedProfiles()
	if err != nil {
		return nil, fmt.Errorf("failed to load translated profiles dataset: %w", err)
	}

	service := &OfficialProfileService{
		cache:         cache,
		modelManager:  modelManager,
		logger:        logger,
		membersData:   membersData,
		profiles:      profiles,
		translations:  preTranslated,
		englishToSlug: make(map[string]string, len(profiles)),
		channelToSlug: make(map[string]string, len(membersData.Members)),
	}

	for slug, profile := range profiles {
		if profile == nil {
			continue
		}
		key := util.NormalizeKey(profile.EnglishName)
		if key != "" {
			service.englishToSlug[key] = slug
		}
	}

	for _, member := range membersData.Members {
		if member == nil {
			continue
		}
		// Prefer direct mapping from dataset if available
		if slug, ok := service.slugForEnglishName(member.Name); ok {
			service.channelToSlug[strings.ToLower(member.ChannelID)] = slug
			continue
		}

		// Attempt to derive slug from known link
		key := util.NormalizeKey(member.Name)
		if key != "" {
			service.englishToSlug[key] = util.Slugify(member.Name)
		}
	}

	logger.Info("OfficialProfileService initialized",
		zap.Int("profiles", len(service.profiles)),
		zap.Int("translated_profiles", len(service.translations)),
		zap.Int("index_english", len(service.englishToSlug)),
		zap.Int("index_channel", len(service.channelToSlug)),
	)

	return service, nil
}

// GetProfileWithTranslation returns the static raw profile and a translated version for a given English name.
func (s *OfficialProfileService) GetProfileWithTranslation(ctx context.Context, englishName string) (*domain.TalentProfile, *domain.TranslatedTalentProfile, error) {
	if strings.TrimSpace(englishName) == "" {
		return nil, nil, fmt.Errorf("멤버 이름이 필요합니다.")
	}

	profile, err := s.GetRawProfileByEnglish(englishName)
	if err != nil {
		return nil, nil, err
	}

	translated, err := s.getTranslatedProfile(ctx, profile)
	if err != nil {
		return nil, nil, err
	}

	return profile, translated, nil
}

// GetRawProfileByEnglish looks up a profile by the official English name.
func (s *OfficialProfileService) GetRawProfileByEnglish(englishName string) (*domain.TalentProfile, error) {
	if profile, ok := s.profileByEnglish(englishName); ok {
		return profile, nil
	}
	return nil, fmt.Errorf("'%s' 멤버의 공식 프로필 정보를 찾을 수 없습니다.", englishName)
}

// GetRawProfileByChannelID retrieves a profile using a Holodex channel ID.
func (s *OfficialProfileService) GetRawProfileByChannelID(channelID string) (*domain.TalentProfile, error) {
	if channelID == "" {
		return nil, fmt.Errorf("channel id is empty")
	}
	slug, ok := s.channelToSlug[strings.ToLower(channelID)]
	if !ok {
		return nil, fmt.Errorf("채널ID '%s'에 대한 공식 프로필이 없습니다.", channelID)
	}
	profile, ok := s.profiles[slug]
	if !ok || profile == nil {
		return nil, fmt.Errorf("'%s' 슬러그에 대한 프로필 데이터가 없습니다.", slug)
	}
	return profile, nil
}

func (s *OfficialProfileService) profileByEnglish(englishName string) (*domain.TalentProfile, bool) {
	slug, ok := s.slugForEnglishName(englishName)
	if !ok {
		return nil, false
	}
	profile, ok := s.profiles[slug]
	if !ok || profile == nil {
		return nil, false
	}
	return profile, true
}

func (s *OfficialProfileService) slugForEnglishName(name string) (string, bool) {
	key := util.NormalizeKey(name)
	if key == "" {
		return "", false
	}
	slug, ok := s.englishToSlug[key]
	return slug, ok
}

func (s *OfficialProfileService) getTranslatedProfile(ctx context.Context, raw *domain.TalentProfile) (*domain.TranslatedTalentProfile, error) {
	if raw == nil {
		return nil, fmt.Errorf("raw profile is nil")
	}

	cacheKey := fmt.Sprintf(cacheKeyProfileTranslated, translationLocale, raw.Slug)

	if s.cache != nil {
		var cached domain.TranslatedTalentProfile
		if err := s.cache.Get(ctx, cacheKey, &cached); err == nil && cached.DisplayName != "" {
			return &cached, nil
		}
	}

	if translated := s.translations[raw.Slug]; translated != nil {
		cloned := cloneTranslatedProfile(translated)
		if s.cache != nil && cloned != nil {
			if err := s.cache.Set(ctx, cacheKey, cloned, 0); err != nil {
				s.logger.Warn("Failed to cache translated profile",
					zap.String("slug", raw.Slug),
					zap.Error(err),
				)
			}
		}
		return cloned, nil
	}

	if s.modelManager == nil {
		return nil, fmt.Errorf("AI 번역 서비스가 설정되지 않았습니다.")
	}

	promptText, err := prompt.BuildProfileTranslationPrompt(prompt.ProfileTranslationPromptVars{
		EnglishName:    raw.EnglishName,
		JapaneseName:   raw.JapaneseName,
		Catchphrase:    raw.Catchphrase,
		Description:    raw.Description,
		DataEntries:    convertToPromptEntries(raw.DataEntries),
		MaxDataEntries: util.Min(len(raw.DataEntries), maxPromptDataEntries),
	})
	if err != nil {
		return nil, fmt.Errorf("번역 프롬프트 생성 실패: %w", err)
	}

	var translated domain.TranslatedTalentProfile
	metadata, genErr := s.modelManager.GenerateJSON(ctx, promptText, PresetBalanced, &translated, nil)
	if genErr != nil {
		return nil, fmt.Errorf("번역 생성 실패: %w", genErr)
	}

	s.logger.Info("Official profile translated",
		zap.String("member", raw.EnglishName),
		zap.String("provider", metadata.Provider),
		zap.String("model", metadata.Model),
		zap.Bool("fallback", metadata.UsedFallback),
	)

	if s.cache != nil {
		if err := s.cache.Set(ctx, cacheKey, translated, 0); err != nil {
			s.logger.Warn("Failed to cache translated profile",
				zap.String("slug", raw.Slug),
				zap.Error(err),
			)
		}
	}

	cloned := cloneTranslatedProfile(&translated)
	if cloned != nil {
		if s.translations == nil {
			s.translations = make(map[string]*domain.TranslatedTalentProfile)
		}
		s.translations[raw.Slug] = cloned
	}

	return cloned, nil
}

func convertToPromptEntries(entries []domain.TalentProfileEntry) []prompt.ProfileTranslationPromptEntry {
	if len(entries) == 0 {
		return []prompt.ProfileTranslationPromptEntry{}
	}

	result := make([]prompt.ProfileTranslationPromptEntry, 0, len(entries))
	for _, entry := range entries {
		label := strings.TrimSpace(entry.Label)
		value := strings.TrimSpace(entry.Value)
		if label == "" || value == "" {
			continue
		}
		result = append(result, prompt.ProfileTranslationPromptEntry{
			Label: label,
			Value: value,
		})
	}
	return result
}

// PreloadTranslations writes pre-translated profiles into cache so first access is instant.
func (s *OfficialProfileService) PreloadTranslations(ctx context.Context) {
	if s == nil || s.cache == nil || len(s.translations) == 0 {
		return
	}

	written := 0
	for slug, profile := range s.translations {
		if profile == nil {
			continue
		}
		if err := s.cache.Set(ctx, fmt.Sprintf(cacheKeyProfileTranslated, translationLocale, slug), profile, 0); err != nil {
			s.logger.Warn("Failed to preload translated profile",
				zap.String("slug", slug),
				zap.Error(err),
			)
			continue
		}
		written++
	}

	if written > 0 {
		s.logger.Info("Preloaded translated profiles",
			zap.Int("count", written))
	}
}

func cloneTranslatedProfile(src *domain.TranslatedTalentProfile) *domain.TranslatedTalentProfile {
	if src == nil {
		return nil
	}

	clone := *src
	if len(src.Highlights) > 0 {
		clone.Highlights = append([]string(nil), src.Highlights...)
	}
	if len(src.Data) > 0 {
		clone.Data = make([]domain.TranslatedProfileDataRow, len(src.Data))
		copy(clone.Data, src.Data)
	}
	return &clone
}
