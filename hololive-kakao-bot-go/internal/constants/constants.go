package constants

import "time"

var CacheTTL = struct {
	LiveStreams      time.Duration
	UpcomingStreams  time.Duration
	ChannelSchedule  time.Duration
	ChannelInfo      time.Duration
	ChannelSearch    time.Duration
	NextStreamInfo   time.Duration
	NotificationSent time.Duration
}{
	LiveStreams:      5 * time.Minute,  // 5분 - 라이브 스트림 목록
	UpcomingStreams:  5 * time.Minute,  // 5분 - 예정 스트림 목록
	ChannelSchedule:  5 * time.Minute,  // 5분 - 채널 스케줄
	ChannelInfo:      20 * time.Minute, // 20분 - 채널 정보
	ChannelSearch:    10 * time.Minute, // 10분 - 채널 검색 결과
	NextStreamInfo:   60 * time.Minute, // 1시간 - 다음 방송 정보
	NotificationSent: 60 * time.Minute, // 1시간 - 알림 발송 기록
}

var WebSocketConfig = struct {
	MaxReconnectAttempts int
	ReconnectDelay       time.Duration
}{
	MaxReconnectAttempts: 5,
	ReconnectDelay:       5 * time.Second,
}

var RedisConfig = struct {
	ReadyTimeout time.Duration
}{
	ReadyTimeout: 5 * time.Second,
}

var AIInputLimits = struct {
	MaxQueryLength int
}{
	MaxQueryLength: 500,
}

var RetryConfig = struct {
	MaxAttempts int
	BaseDelay   time.Duration
	Jitter      time.Duration
}{
	MaxAttempts: 3,
	BaseDelay:   500 * time.Millisecond,
	Jitter:      250 * time.Millisecond,
}

var CircuitBreakerConfig = struct {
	FailureThreshold    int
	ResetTimeout        time.Duration
	RateLimitTimeout    time.Duration
	HealthCheckInterval time.Duration
	HealthCheckTimeout  time.Duration
}{
	FailureThreshold:    3,                // 3회 연속 실패 시 Circuit OPEN
	ResetTimeout:        30 * time.Second, // 기본 재시도 대기 시간 (30초)
	RateLimitTimeout:    1 * time.Hour,    // 429 Rate Limit 전용 타임아웃 (1시간)
	HealthCheckInterval: 10 * time.Minute, // Health Check 주기 (10분)
	HealthCheckTimeout:  10 * time.Second, // Health Check 타임아웃 (10초)
}

var PaginationConfig = struct {
	ItemsPerPage   int
	Timeout        time.Duration
	MaxEmbedFields int
}{
	ItemsPerPage:   10,              // 페이지당 항목 수
	Timeout:        3 * time.Minute, // 페이지네이션 타임아웃
	MaxEmbedFields: 25,              // Discord Embed 필드 최대 개수
}

var APIConfig = struct {
	HolodexBaseURL   string
	HolodexTimeout   time.Duration
	MaxRetryAttempts int
}{
	HolodexBaseURL:   "https://holodex.net/api/v2",
	HolodexTimeout:   10 * time.Second,
	MaxRetryAttempts: 3,
}

var StringLimits = struct {
	EmbedTitle       int
	EmbedDescription int
	EmbedFieldName   int
	EmbedFieldValue  int
	StreamTitle      int
	NextStreamTitle  int
}{
	EmbedTitle:       256,
	EmbedDescription: 4096,
	EmbedFieldName:   256,
	EmbedFieldValue:  1024,
	StreamTitle:      100,
	NextStreamTitle:  40,
}
