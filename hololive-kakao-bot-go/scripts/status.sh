#!/bin/bash
# Hololive KakaoTalk Bot (Go) 상태 확인 스크립트 v1.0

PROJECT_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_ROOT"

PID_FILE=".bot.pid"

echo "[STATUS] Hololive KakaoTalk Bot (Go) Status"
echo "========================================"

# === 1. 봇 프로세스 상태 ===
if [ -f "$PID_FILE" ]; then
  BOT_PID=$(cat "$PID_FILE")

  if ps -p "$BOT_PID" > /dev/null 2>&1; then
    UPTIME=$(ps -o etime= -p "$BOT_PID" | tr -d ' ')
    MEM=$(ps -o rss= -p "$BOT_PID" | awk '{printf "%.1f MB", $1/1024}')
    CPU=$(ps -o %cpu= -p "$BOT_PID" | tr -d ' ')

    echo "Bot Status: [RUNNING]"
    echo "  PID: $BOT_PID"
    echo "  Uptime: $UPTIME"
    echo "  Memory: $MEM"
    echo "  CPU: ${CPU}%"
  else
    echo "Bot Status: [STOPPED] (stale PID file)"
    echo "  Stale PID: $BOT_PID"
  fi
else
  # Fallback: 작업 디렉토리 기반 검색
  FALLBACK_PIDS=$(pgrep -f "bin/bot" 2>/dev/null | while read pid; do
    dir=$(readlink -f /proc/$pid/cwd 2>/dev/null || echo "")
    if [ "$dir" = "$PROJECT_ROOT" ]; then
      echo "$pid"
    fi
  done)

  if [ -n "$FALLBACK_PIDS" ]; then
    echo "Bot Status: [WARN] RUNNING (no PID file)"
    echo "  PIDs: $FALLBACK_PIDS"
    echo "  Warning: Use ./scripts/start-bot.sh to manage with PID file"
  else
    echo "Bot Status: [NOT RUNNING]"
  fi
fi

echo ""
echo "Dependencies:"
echo "-------------"

# === 2. Redis 상태 ===
if docker ps | grep "holo-redis" | grep -q "Up"; then
  if timeout 2 docker exec holo-redis redis-cli ping > /dev/null 2>&1; then
    REDIS_PORT=$(grep "^REDIS_PORT=" .env 2>/dev/null | cut -d'=' -f2 || echo "6380")
    echo "Redis: [CONNECTED] (host port $REDIS_PORT -> container port 6379)"
  else
    echo "Redis: [WARN] CONTAINER UP but not responding"
  fi
else
  echo "Redis: [NOT RUNNING]"
fi

# === 3. Iris 서버 상태 ===
IRIS_PORT=$(grep "^IRIS_BASE_URL=" .env 2>/dev/null | cut -d'=' -f2 | grep -oP ':\K\d+' || echo "3000")

if ss -tuln | grep -q ":$IRIS_PORT "; then
  echo "Iris: [LISTENING] (port $IRIS_PORT)"
else
  echo "Iris: [NOT LISTENING] (port $IRIS_PORT)"
fi

# === 4. 로그 파일 ===
echo ""
echo "Logs:"
echo "-----"
if [ -f "logs/bot.log" ]; then
  LOG_SIZE=$(du -h logs/bot.log 2>/dev/null | cut -f1)
  LOG_LINES=$(wc -l < logs/bot.log 2>/dev/null)
  echo "Application: logs/bot.log ($LOG_SIZE, $LOG_LINES lines)"
fi

if [ -f "logs/nohup.log" ]; then
  NOHUP_SIZE=$(du -h logs/nohup.log 2>/dev/null | cut -f1)
  NOHUP_LINES=$(wc -l < logs/nohup.log 2>/dev/null)
  echo "Process: logs/nohup.log ($NOHUP_SIZE, $NOHUP_LINES lines)"
fi

echo ""
echo "Commands:"
echo "---------"
echo "Start:   ./scripts/start-bot.sh"
echo "Stop:    ./scripts/stop-bot.sh"
echo "Restart: ./scripts/restart-bot.sh [-b|--build]"
echo "Rebuild: ./scripts/rebuild.sh"
echo "Status:  ./scripts/status.sh"
