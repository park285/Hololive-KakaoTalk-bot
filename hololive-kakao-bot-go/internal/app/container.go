package app

import (
	"context"
	"fmt"
	"time"

	"github.com/kapu/hololive-kakao-bot-go/internal/adapter"
	"github.com/kapu/hololive-kakao-bot-go/internal/bot"
	"github.com/kapu/hololive-kakao-bot-go/internal/config"
	"github.com/kapu/hololive-kakao-bot-go/internal/iris"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/ai"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/cache"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/database"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/holodex"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/matcher"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/member"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/notification"
	"github.com/kapu/hololive-kakao-bot-go/internal/service/youtube"
	"go.uber.org/zap"
)

// Container bundles assembled services for constructing runtime components like Bot.
type Container struct {
	Config *config.Config
	Logger *zap.Logger

	botDeps *bot.Dependencies
}

// NewBot instantiates a bot using the pre-built dependency graph.
func (c *Container) NewBot() (*bot.Bot, error) {
	if c == nil || c.botDeps == nil {
		return nil, fmt.Errorf("bot dependencies not initialized")
	}
	return bot.NewBot(c.botDeps)
}

// Build assembles all infrastructure services and returns a container capable of
// creating fully-wired bots. All heavy-weight initialization (DB/cache/AI) is
// performed here so that bot.NewBot stays focused on orchestration logic.
func Build(ctx context.Context, cfg *config.Config, logger *zap.Logger) (container *Container, err error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}
	if logger == nil {
		return nil, fmt.Errorf("logger must not be nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	var closers []func()
	defer func() {
		if err != nil {
			for i := len(closers) - 1; i >= 0; i-- {
				closers[i]()
			}
		}
	}()

	// Messaging primitives
	irisClient := iris.NewClient(cfg.Iris.BaseURL, logger)
	irisWS := iris.NewWebSocket(cfg.Iris.WSURL, 5, 5*time.Second, logger)
	messageAdapter := adapter.NewMessageAdapter(cfg.Bot.Prefix)
	formatter := adapter.NewResponseFormatter(cfg.Bot.Prefix)

	// Cache and database
	cacheSvc, err := cache.NewCacheService(cache.CacheConfig{
		Host:     cfg.Redis.Host,
		Port:     cfg.Redis.Port,
		Password: cfg.Redis.Password,
		DB:       cfg.Redis.DB,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache service: %w", err)
	}
	closers = append(closers, func() {
		_ = cacheSvc.Close()
	})

	postgresSvc, err := database.NewPostgresService(database.PostgresConfig{
		Host:     cfg.Postgres.Host,
		Port:     cfg.Postgres.Port,
		User:     cfg.Postgres.User,
		Password: cfg.Postgres.Password,
		Database: cfg.Postgres.Database,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create postgres service: %w", err)
	}
	closers = append(closers, func() {
		_ = postgresSvc.Close()
	})

	// Member domain setup
	memberRepo := member.NewMemberRepository(postgresSvc, logger)
	memberCache, err := member.NewMemberCache(memberRepo, cacheSvc.GetRedisClient(), logger, member.MemberCacheConfig{
		WarmUp:   true,
		RedisTTL: 30 * time.Minute,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create member cache: %w", err)
	}

	membersData := member.NewMemberServiceAdapter(memberCache)
	members := membersData.GetAllMembers()
	logger.Info("Member data loaded from PostgreSQL",
		zap.Int("total_members", len(members)))

	memberMap := make(map[string]string, len(members))
	for _, m := range members {
		if m != nil && m.ChannelID != "" {
			memberMap[m.Name] = m.ChannelID
		}
	}

	if err := cacheSvc.InitializeMemberDatabase(ctx, memberMap); err != nil {
		return nil, fmt.Errorf("failed to initialize member cache: %w", err)
	}

	// Holodex services
	scraper := holodex.NewScraperService(cacheSvc, membersData, logger)
	holodexSvc, err := holodex.NewHolodexService(cfg.Holodex.APIKeys, cacheSvc, scraper, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create holodex service: %w", err)
	}

	// AI stack
	modelManager, err := ai.NewModelManager(ctx, ai.ModelManagerConfig{
		GeminiAPIKey:       cfg.Gemini.APIKey,
		OpenAIAPIKey:       cfg.OpenAI.APIKey,
		DefaultGeminiModel: "gemini-2.5-flash",
		DefaultOpenAIModel: "gpt-5-mini",
		EnableFallback:     cfg.OpenAI.EnableFallback,
	}, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create model manager: %w", err)
	}

	geminiSvc := ai.NewGeminiService(modelManager, logger)
	closers = append(closers, func() {
		geminiSvc.Close()
	})

	if err := geminiSvc.InitializeMemberListCache(ctx, membersData); err != nil {
		logger.Warn("Failed to initialize member list cache, will use inline member list", zap.Error(err))
	} else {
		logger.Info("Member list context cache initialized successfully")
	}

	profileSvc, err := member.NewProfileService(cacheSvc, membersData, modelManager, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create official profile service: %w", err)
	}
	profileSvc.PreloadTranslations(ctx)

	matcherSvc := matcher.NewMemberMatcher(membersData, cacheSvc, holodexSvc, geminiSvc, logger, ctx)
	alarmSvc := notification.NewAlarmService(cacheSvc, holodexSvc, logger, cfg.Notification.AdvanceMinutes)

	var (
		youtubeSvc       *youtube.YouTubeService
		youtubeStatsRepo *youtube.YouTubeStatsRepository
		youtubeScheduler *youtube.YouTubeScheduler
	)

	if cfg.YouTube.EnableQuotaBuilding && cfg.YouTube.APIKey != "" {
		ytSvc, ytErr := youtube.NewYouTubeService(cfg.YouTube.APIKey, cacheSvc, logger)
		if ytErr != nil {
			logger.Warn("Failed to initialize YouTube service (optional feature)", zap.Error(ytErr))
		} else {
			youtubeSvc = ytSvc
			youtubeStatsRepo = youtube.NewYouTubeStatsRepository(postgresSvc, logger)
			youtubeScheduler = youtube.NewYouTubeScheduler(youtubeSvc, cacheSvc, youtubeStatsRepo, membersData, logger)
			logger.Info("YouTube quota building enabled",
				zap.String("mode", "API Key"),
				zap.Int("daily_target", 9192))
		}
	}

	deps := &bot.Dependencies{
		Config:           cfg,
		Logger:           logger,
		IrisClient:       irisClient,
		IrisWebSocket:    irisWS,
		MessageAdapter:   messageAdapter,
		Formatter:        formatter,
		Cache:            cacheSvc,
		Postgres:         postgresSvc,
		Holodex:          holodexSvc,
		YouTubeService:   youtubeSvc,
		YouTubeStats:     youtubeStatsRepo,
		YouTubeScheduler: youtubeScheduler,
		Gemini:           geminiSvc,
		Profiles:         profileSvc,
		Alarm:            alarmSvc,
		Matcher:          matcherSvc,
		MembersData:      membersData,
	}

	return &Container{
		Config:  cfg,
		Logger:  logger,
		botDeps: deps,
	}, nil
}
