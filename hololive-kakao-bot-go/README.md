# Hololive KakaoTalk Bot (Go)

> 홀로라이브 VTuber 스케줄 및 정보 제공 카카오톡 봇 (Go 버전)

카카오톡을 통해 홀로라이브 소속 VTuber들의 스케줄, 프로필 정보를 제공하고 자연어 질의응답을 지원하는 봇입니다.

## 주요 기능

- **스케줄 조회**: Holodex API 연동으로 실시간 방송 및 예정 스케줄 확인
- **멤버 정보**: 공식 프로필 데이터 제공 (AI 번역 지원)
- **자연어 처리**: Gemini API를 통한 한국어 질의응답
- **알림 설정**: 사용자별 멤버 알림 관리
- **캐싱**: Redis 기반 고성능 캐싱
- **Circuit Breaker**: AI API 장애 대응

## 빠른 시작

### 요구사항

- Go 1.24+
- Redis 서버
- Iris Messenger 서버
- Gemini API Key (필수), OpenAI API Key (선택)

### 빌드

```bash
cd /home/kapu/gemini/hololive-kakao-bot-go
go build -o bin/bot ./cmd/bot
```

### 실행

```bash
# .env 파일 생성 (템플릿 참고)
cp .env.example .env
nano .env  # API 키 및 설정 입력

# 봇 실행
./bin/bot

# 또는 스크립트 사용
./scripts/start-bot.sh
```

### 운영 스크립트

```bash
./scripts/start-bot.sh    # 봇 시작
./scripts/stop-bot.sh     # 봇 종료
./scripts/restart-bot.sh  # 봇 재시작
./scripts/rebuild.sh      # 재빌드 및 재시작
./scripts/status.sh       # 봇 상태 확인
```

## 프로젝트 구조

```
hololive-kakao-bot-go/
├── cmd/
│   ├── bot/                      # 메인 봇 엔트리포인트
│   └── tools/                    # 데이터 관리 도구
│       ├── fetch_profiles/       # 공식 프로필 fetch
│       └── translate_profiles/   # 프로필 번역
├── internal/
│   ├── adapter/                  # 메시지 포맷팅
│   ├── bot/                      # 봇 오케스트레이션
│   ├── command/                  # 명령어 핸들러 (10개)
│   ├── config/                   # 환경 설정
│   ├── constants/                # 상수 정의
│   ├── domain/                   # 도메인 모델
│   ├── iris/                     # Iris Messenger 클라이언트
│   ├── prompt/                   # AI 프롬프트
│   ├── service/                  # 비즈니스 로직
│   └── util/                     # 헬퍼 함수
├── data/                         # 임베디드 정적 데이터
│   ├── members.json              # 멤버 정보
│   ├── official_profiles/*.json  # 공식 프로필
│   └── official_translated/*.json# 번역된 프로필
├── scripts/                      # 운영 스크립트
├── migrations/                   # DB 마이그레이션
└── bin/bot                       # 빌드된 바이너리
```

## 환경 변수 설정

`.env` 파일 또는 시스템 환경 변수로 설정:

```env
# Iris Server
IRIS_BASE_URL=http://localhost:3000
IRIS_WS_URL=ws://localhost:3000/ws

# KakaoTalk
KAKAO_ROOMS=홀로라이브 알림방

# Holodex API (여러 키 로테이션 지원)
HOLODEX_API_KEY_1=your_key_here
HOLODEX_API_KEY_2=your_second_key  # Optional
HOLODEX_API_KEY_3=your_third_key   # Optional

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
REDIS_DB=0

# AI Services
GEMINI_API_KEY=your_gemini_key
OPENAI_API_KEY=your_openai_key  # Optional fallback

# Logging
LOG_LEVEL=info              # debug, info, warn, error
LOG_FILE=logs/bot.log
```

## 지원 명령어

| 명령어 | 설명 | 예시 |
|--------|------|------|
| `!라이브` | 현재 생방송 목록 | `!라이브` |
| `!예정` | 향후 24시간 스케줄 | `!예정` |
| `!일정 [멤버]` | 특정 멤버 스케줄 | `!일정 페코라` |
| `!알림 [멤버]` | 알림 등록/해제 | `!알림 미코` |
| `!멤버정보 [멤버]` | 멤버 프로필 정보 | `!멤버정보 수이세이` |
| `!ask [질문]` | 자연어 질의응답 | `!ask 페코라 알려줘` |
| `!도움말` | 명령어 목록 | `!도움말` |

## 기술 스택

- **언어**: Go 1.24
- **AI**: Google Gemini API, OpenAI API (fallback)
- **캐시**: Redis
- **로깅**: Uber Zap
- **메신저**: Iris (KakaoTalk 연동)
- **데이터**: Holodex API

## 아키텍처 특징

- **Circuit Breaker**: AI API 장애 시 자동 fallback
- **Rate Limiting**: Holodex API 요청 제한 준수
- **캐싱 전략**: Redis 다층 캐싱으로 API 호출 최소화
- **임베디드 데이터**: 공식 프로필 정적 데이터 내장
- **Context Caching**: Gemini 멤버 리스트 캐싱으로 비용 절감

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/...

# 특정 패키지 테스트
go test ./internal/domain -v

# 커버리지 확인
go test -cover ./internal/...
```

## 라이선스

Private Repository

---
