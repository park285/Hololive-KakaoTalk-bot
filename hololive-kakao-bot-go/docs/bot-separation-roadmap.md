# Hololive Kakao Bot – 봇 분리 로드맵

## 1. 현황 요약
- **기존 구조**: 단일 프로세스가 Kakao 메시지 처리, Holodex/YouTube 데이터 수집, Gemini 기반 질의 응답을 모두 담당.
- **해결된 선행 이슈**
  - YouTube 백업 캐시 키 충돌 → 채널 ID 정렬 기반 키로 수정. (`internal/service/youtube/service.go`)
  - 스케줄러 zero-division/빈 배치 → 방어 로직 추가. (`internal/service/youtube/scheduler.go`)
  - Gemini 멤버 리스트 캐시가 초기화 직후 중단되던 문제 → 장수 컨텍스트 도입. (`internal/app/container.go`)
  - `CommandType.IsValid()`에서 `CommandStats` 누락 → 재등록. (`internal/domain/command.go`)
- **남은 제약**
  - 실시간 경로와 LLM/데이터 파이프라인이 동일 배포 단위에 묶여 있어 재시작·배포 단일점 장애가 존재.
  - Redis/DB 계약, 세션 헤더(`X-User-Id`, `X-User-Email`, `X-Session-Id`) 전달 규칙이 서비스별로 명확히 정리되어 있지 않음.

## 2. 목표
1. **실시간 코어 봇**: Kakao 메시지 처리, 명령 디스패치, 알림 발송, LLM 프록시 호출만 담당.
2. **데이터 인젝션 서비스**: YouTube quota 빌더, Holodex/프로필 동기화, 캐시/DB 갱신을 배치 워커로 운영.
3. **AI 서비스**: Gemini/OpenAI 호출을 API 형태로 노출하여 프롬프트 관리·실험을 독립적으로 수행.
4. 공통 규약(세션 헤더, Redis 키, Postgres 스키마, 관측 지표)을 문서화하고, 서비스별 장애가 서로에게 전파되지 않도록 격리.

## 3. 아키텍처 개요
### 3.1 서비스 분리안
| 서비스 | 주요 책임 | 입출력 | 공통 의존성 |
| --- | --- | --- | --- |
| Core Bot (`bot-core`) | WebSocket 수신, 명령 라우팅, 알람/통계 응답 | Kakao ↔ Core Bot ↔ AI/인젝션 API | Redis, Postgres, Iris |
| Ingestion Worker (`bot-ingestion`) | Holodex/YouTube/멤버 데이터 수집, Redis/DB 업데이트, quota 모니터링 | 외부 API ↔ Ingestion ↔ Redis/Postgres | Redis, Postgres, Scheduler |
| AI Gateway (`bot-ai`) | Gemini/OpenAI 호출, 프롬프트 관리, 회로 차단기 | Core Bot ↔ AI ↔ Gemini/OpenAI | Redis (세션 캐시), Secret Vault |

### 3.2 데이터·세션 계약
- **Headers**: 모든 내부 API는 `X-User-Id`, `X-User-Email`, `X-Session-Id` 필수. Core Bot에서 검증 후 전달.
- **Redis 키** (예시): `hololive:members`, `member:name:*`, `youtube:recent_videos:*`, `alarm:schedule:*`. Ingestion이 주도적으로 갱신하고 Core Bot은 읽기 전용.
- **Postgres 테이블**: `youtube_stats_history`, `youtube_stats_changes`, `youtube_milestones` 등 통계 테이블은 Ingestion이 upsert, Core Bot은 조회 전용.
- **Pub/Sub / 이벤트** (선택): Ingestion이 캐시 갱신 완료 시 `cache_update` 채널에 이벤트를 발행 → Core Bot이 필요한 경우 실시간 리프레시.

## 4. 단계별 로드맵
### Phase 0 – 추가 하드닝 & 문서화
- Redis/DB 스키마, 캐시 TTL, 백오프 정책을 `docs/data-contracts.md`로 정리.
- Core Bot의 에러 메시지 가드, 워커 풀 설정(Ask, Stats 등)을 리뷰하여 SLA 명시.
- docker compose 프로필 구조 설계 (`bot-core`, `bot-ingestion`, `bot-ai`).

### Phase 1 – Ingestion 서비스 추출
- `internal/service/youtube/*`, `internal/service/holodex/*`, `internal/service/member/*` 재사용하여 CLI/서비스 스켈레톤 작성.
- 작업 큐(또는 cron) 스케줄 정의: Holodex(1분), YouTube 통계(12시간), 멤버 프로필(24시간).
- 배포: `docker compose --profile ingestion up`으로 독립 실행 가능하게 구성.
- 관측: API 호출 실패율, 캐시 갱신 시간, 백오프 횟수를 Prometheus로 수집.

### Phase 2 – AI Gateway 구축
- `internal/service/ai` + `internal/prompt` 모듈을 HTTP/gRPC API로 감싸 `/api/ai/{model}/{action}` 규약 정의.
- 요청 시 세션 헤더 검증 → Redis로 세션 확인 → Gemini/OpenAI 호출 → JSON 응답.
- 프롬프트/모델 설정은 YAML/JSON 리소스로 외부화. 캐시 초기화, 회로 차단기 상태 Endpoints 제공.
- Core Bot은 해당 API를 호출하도록 Ask/Clarification 경로를 리팩터링.

### Phase 3 – Core Bot 슬림화
- Core Bot에서 데이터 수집 로직 제거, Ingestion/AI API 클라이언트만 유지.
- 명령 실행 시 API 실패 대비 fallback 메시지/재시도 정책 정리.
- 배포 파이프라인 분리: Core Bot은 빠른 배포/롤백, Ingestion·AI는 예약 배포.

### Phase 4 – 확장 및 정비
- 필요 시 추가 외부 데이터 소스(Bilibili 등) ← Ingestion 패턴 재활용.
- 이벤트 기반 알림(예: Redis 스트림, Kafka)으로 확장 가능성 검토.
- SLO/SLI 정의: Core Bot 응답 지연, Ingestion 실행 성공률, AI 응답 실패율.

## 5. 체크리스트
- [ ] Redis/DB 키·스키마 계약 문서화 및 공유.
- [ ] docker compose 프로필 작성 (`bot-core`, `bot-ingestion`, `bot-ai`).
- [ ] Ingestion 서비스 초기 구현 + 스케줄/백오프 설정.
- [ ] AI Gateway API 설계서 & 인증 전략 확정.
- [ ] Core Bot → AI/인젝션 API 연동 코드 경로 확정.
- [ ] 관측/알람 대시보드 (Prometheus/Grafana) 구성.
- [ ] 배포·롤백 절차 및 비상 연락망 정리.

## 6. 참고 파일
- `docs/bot-decomposition-plan.md`: 과거 기능 분리 구상 및 런타임 토폴로지.
- `docs/bot-monolith-assessment.md`: 모놀리스 병목 분석.
- 최신 수정 커밋: `Fix YouTube backup cache key uniqueness`, `Harden scheduler guardrails`.
