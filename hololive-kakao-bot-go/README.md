# Hololive KakaoTalk Bot (Go)

> Go 포팅 버전 - Phase 1 완료

## 프로젝트 상태

**현재**: Phase 1 완료 (인프라 구축)
**다음**: Phase 2 시작 (도메인 모델 + 타입)

### Phase 1 완료 항목 ✅

- [x] Go 모듈 초기화
- [x] 디렉토리 구조 생성
- [x] 핵심 의존성 설치
  - `google.golang.org/genai` v1.28.0
  - `github.com/openai/openai-go/v3` v3.2.0
  - `github.com/redis/go-redis/v9` v9.14.0
  - `go.uber.org/zap` v1.27.0
  - `github.com/gorilla/websocket` v1.5.3
- [x] Config 로더 구현
- [x] Zap Logger 구현
- [x] 기본 main.go 작성
- [x] 빌드 검증 성공 (5.9MB 바이너리)

## 빠른 시작

### 요구사항

- Go 1.24+ (자동 설치됨)
- Redis 서버
- Iris 서버

### 빌드

```bash
cd /home/kapu/gemini/hololive-kakao-bot-go
go build -o bin/bot ./cmd/bot
```

### 실행

```bash
# .env 파일 생성
cp .env.example .env
nano .env  # 설정 수정

# 봇 실행
./bin/bot
```

## 프로젝트 구조

```
hololive-kakao-bot-go/
├── cmd/bot/              # 메인 엔트리포인트
│   └── main.go
├── internal/
│   ├── adapter/          # 어댑터 레이어 (미구현)
│   ├── command/          # 명령어 핸들러 (미구현)
│   ├── config/           # ✅ 설정 관리
│   │   └── config.go
│   ├── domain/           # 도메인 모델 (미구현)
│   ├── iris/             # Iris 클라이언트 (미구현)
│   ├── prompt/           # AI 프롬프트 (미구현)
│   ├── service/          # 비즈니스 로직 (미구현)
│   └── util/             # ✅ 유틸리티
│       └── logger.go
├── pkg/errors/           # 에러 타입 (미구현)
├── data/                 # 정적 데이터 (미구현)
├── bin/                  # 빌드 아티팩트
│   └── bot              # ✅ 5.9MB 바이너리
├── .env.example          # 환경 변수 템플릿
├── go.mod
└── README.md
```

## 설정

환경 변수는 `.env` 파일 또는 시스템 환경 변수로 설정:

```env
# Iris Server
IRIS_BASE_URL=http://localhost:3000
IRIS_WS_URL=ws://localhost:3000/ws

# KakaoTalk
KAKAO_ROOMS=홀로라이브 알림방

# Holodex API
HOLODEX_API_KEY_1=your_key_here

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# AI Services
GEMINI_API_KEY=your_key_here
OPENAI_API_KEY=your_key_here  # Optional fallback

# Logging
LOG_LEVEL=info
LOG_FILE=logs/bot.log
```

## 다음 단계 (Phase 2)

- [ ] `domain/stream.go` - Stream 모델
- [ ] `domain/channel.go` - Channel 모델
- [ ] `domain/alarm.go` - Alarm 모델
- [ ] `pkg/errors/errors.go` - 에러 타입
- [ ] 상수 정의
- [ ] `data/members.json` 파싱

## 포팅 진행률

```
Phase 1: ████████████████████ 100% (완료)
Phase 2: ░░░░░░░░░░░░░░░░░░░░   0%
Phase 3: ░░░░░░░░░░░░░░░░░░░░   0%
...
전체:    ██░░░░░░░░░░░░░░░░░░  12.5%
```

## 기술 스택

- **언어**: Go 1.24
- **AI**: Gemini API, OpenAI API
- **캐시**: Redis
- **로깅**: Uber Zap
- **환경 변수**: godotenv

---

**원본 프로젝트**: [hololive-kakao-bot](../hololive-kakao-bot) (TypeScript)
**포팅 계획**: [GO_PORTING_PLAN.md](../hololive-kakao-bot/GO_PORTING_PLAN.md)
