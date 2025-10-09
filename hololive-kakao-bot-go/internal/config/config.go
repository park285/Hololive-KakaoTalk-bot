package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	Iris         IrisConfig
	Kakao        KakaoConfig
	Holodex      HolodexConfig
	YouTube      YouTubeConfig
	Redis        RedisConfig
	Gemini       GeminiConfig
	OpenAI       OpenAIConfig
	Notification NotificationConfig
	Logging      LoggingConfig
	Bot          BotConfig
}

type IrisConfig struct {
	BaseURL string
	WSURL   string
}

type KakaoConfig struct {
	Rooms []string
}

type HolodexConfig struct {
	APIKeys []string
}

type YouTubeConfig struct {
	APIKey              string
	EnableQuotaBuilding bool
}

type RedisConfig struct {
	Host     string
	Port     int
	Password string
	DB       int
}

type GeminiConfig struct {
	APIKey string
}

type OpenAIConfig struct {
	APIKey         string
	EnableFallback bool
}

type NotificationConfig struct {
	AdvanceMinutes []int
	CheckInterval  time.Duration
}

type LoggingConfig struct {
	Level string
	File  string
}

type BotConfig struct {
	Prefix string
}

func Load() (*Config, error) {
	_ = godotenv.Load()

	cfg := &Config{
		Iris: IrisConfig{
			BaseURL: getEnv("IRIS_BASE_URL", "http://localhost:3000"),
			WSURL:   getEnv("IRIS_WS_URL", "ws://localhost:3000/ws"),
		},
		Kakao: KakaoConfig{
			Rooms: parseCommaSeparated(getEnv("KAKAO_ROOMS", "홀로라이브 알림방")),
		},
		Holodex: HolodexConfig{
			APIKeys: collectAPIKeys("HOLODEX_API_KEY_"),
		},
		YouTube: YouTubeConfig{
			APIKey:              getEnv("YOUTUBE_API_KEY", ""),
			EnableQuotaBuilding: getEnvBool("YOUTUBE_ENABLE_QUOTA_BUILDING", false),
		},
		Redis: RedisConfig{
			Host:     getEnv("REDIS_HOST", "localhost"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		Gemini: GeminiConfig{
			APIKey: getEnv("GEMINI_API_KEY", ""),
		},
		OpenAI: OpenAIConfig{
			APIKey:         getEnv("OPENAI_API_KEY", ""),
			EnableFallback: getEnvBool("OPENAI_ENABLE_FALLBACK", true),
		},
		Notification: NotificationConfig{
			AdvanceMinutes: parseIntList(getEnv("NOTIFICATION_ADVANCE_MINUTES", "5,15,30")),
			CheckInterval:  time.Duration(getEnvInt("CHECK_INTERVAL_SECONDS", 60)) * time.Second,
		},
		Logging: LoggingConfig{
			Level: getEnv("LOG_LEVEL", "info"),
			File:  getEnv("LOG_FILE", "logs/bot.log"),
		},
		Bot: BotConfig{
			Prefix: getEnv("BOT_PREFIX", "!"),
		},
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.Iris.BaseURL == "" {
		return fmt.Errorf("IRIS_BASE_URL is required")
	}
	if c.Iris.WSURL == "" {
		return fmt.Errorf("IRIS_WS_URL is required")
	}
	if len(c.Kakao.Rooms) == 0 {
		return fmt.Errorf("KAKAO_ROOMS is required")
	}
	if len(c.Holodex.APIKeys) == 0 {
		return fmt.Errorf("at least one HOLODEX_API_KEY is required")
	}
	if c.Gemini.APIKey == "" {
		return fmt.Errorf("GEMINI_API_KEY is required")
	}
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolVal, err := strconv.ParseBool(value); err == nil {
			return boolVal
		}
	}
	return defaultValue
}

func parseCommaSeparated(value string) []string {
	if value == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func parseIntList(value string) []int {
	if value == "" {
		return []int{}
	}
	parts := strings.Split(value, ",")
	result := make([]int, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			if intVal, err := strconv.Atoi(trimmed); err == nil {
				result = append(result, intVal)
			}
		}
	}
	return result
}

func collectAPIKeys(prefix string) []string {
	keys := make([]string, 0)
	for i := 1; i <= 5; i++ {
		envKey := fmt.Sprintf("%s%d", prefix, i)
		if value := os.Getenv(envKey); value != "" {
			keys = append(keys, value)
		}
	}
	return keys
}
