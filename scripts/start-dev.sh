#!/usr/bin/env sh
set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
PROJECT_DIR=$(dirname "$SCRIPT_DIR")

if [ -f "$PROJECT_DIR/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$PROJECT_DIR/.env"
  set +a
fi

RUNTIME_DIR=${SEAL_RUNTIME_DIR:-"$PROJECT_DIR/tmp/dev"}
API_URL=${API_URL:-http://localhost:8080}
WORKER_URL=${WORKER_URL:-http://localhost:8090}
WEB_URL=${WEB_URL:-http://localhost:5173}

API_PID=""
WORKER_PID=""
WEB_PID=""
STOPPING=false

require_command() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf '缺少命令：%s\n' "$1" >&2
    exit 1
  fi
}

port_in_use() {
  port=$1
  if command -v lsof >/dev/null 2>&1; then
    lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1
    return
  fi
  if command -v nc >/dev/null 2>&1; then
    nc -z 127.0.0.1 "$port" >/dev/null 2>&1
    return
  fi
  return 1
}

stop_services() {
  if [ "$STOPPING" = true ]; then
    return
  fi
  STOPPING=true
  trap - INT TERM EXIT
  printf '\n正在停止本地服务…\n'
  for pid in "$WEB_PID" "$API_PID" "$WORKER_PID"; do
    if [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  for pid in "$WEB_PID" "$API_PID" "$WORKER_PID"; do
    if [ -n "$pid" ]; then
      wait "$pid" 2>/dev/null || true
    fi
  done
  rm -f "$RUNTIME_DIR/api.pid" "$RUNTIME_DIR/worker.pid" "$RUNTIME_DIR/web.pid"
  printf '本地服务已停止。\n'
}

show_failure() {
  name=$1
  log_file=$2
  printf '%s 启动失败，最近日志：\n' "$name" >&2
  tail -n 30 "$log_file" >&2 || true
  exit 1
}

wait_for_url() {
  name=$1
  url=$2
  pid=$3
  log_file=$4
  attempts=0
  while [ "$attempts" -lt 60 ]; do
    if curl -fsS "$url" >/dev/null 2>&1; then
      printf '✓ %s 已就绪：%s\n' "$name" "$url"
      return
    fi
    if ! kill -0 "$pid" 2>/dev/null; then
      show_failure "$name" "$log_file"
    fi
    attempts=$((attempts + 1))
    sleep 0.5
  done
  show_failure "$name" "$log_file"
}

require_command go
require_command node
require_command npm
require_command curl

for port in 8080 8090 5173; do
  if port_in_use "$port"; then
    printf '端口 %s 已被占用，请先停止对应进程。\n' "$port" >&2
    exit 1
  fi
done

mkdir -p "$RUNTIME_DIR"
: > "$RUNTIME_DIR/api.log"
: > "$RUNTIME_DIR/worker.log"
: > "$RUNTIME_DIR/web.log"

if [ ! -d "$PROJECT_DIR/apps/web/node_modules" ]; then
  printf '安装 Web 依赖…\n'
  (cd "$PROJECT_DIR/apps/web" && npm ci)
fi
if [ ! -d "$PROJECT_DIR/apps/worker/node_modules" ]; then
  printf '安装 Worker 依赖…\n'
  (cd "$PROJECT_DIR/apps/worker" && npm ci)
fi

trap stop_services INT TERM EXIT

printf '构建 Go API…\n'
(
  cd "$PROJECT_DIR/apps/api"
  GOPROXY=${GOPROXY:-http://127.0.0.1:8080,direct} go build -o "$RUNTIME_DIR/seal-api" ./cmd/server
)

printf '启动 Go API…\n'
(
  APP_RASTERIZER_URL=${APP_RASTERIZER_URL:-http://localhost:8090} \
  APP_DATA_DIR=${APP_DATA_DIR:-"$RUNTIME_DIR/data"} \
  exec "$RUNTIME_DIR/seal-api"
) >"$RUNTIME_DIR/api.log" 2>&1 &
API_PID=$!
printf '%s\n' "$API_PID" > "$RUNTIME_DIR/api.pid"

printf '启动 PNG Worker…\n'
(cd "$PROJECT_DIR/apps/worker" && exec node server.mjs) >"$RUNTIME_DIR/worker.log" 2>&1 &
WORKER_PID=$!
printf '%s\n' "$WORKER_PID" > "$RUNTIME_DIR/worker.pid"

printf '启动 React Web…\n'
(cd "$PROJECT_DIR/apps/web" && exec ./node_modules/.bin/vite --host localhost) >"$RUNTIME_DIR/web.log" 2>&1 &
WEB_PID=$!
printf '%s\n' "$WEB_PID" > "$RUNTIME_DIR/web.pid"

wait_for_url "Worker" "$WORKER_URL/health" "$WORKER_PID" "$RUNTIME_DIR/worker.log"
wait_for_url "API" "$API_URL/api/v1/health" "$API_PID" "$RUNTIME_DIR/api.log"
wait_for_url "Web" "$WEB_URL" "$WEB_PID" "$RUNTIME_DIR/web.log"

printf '\n印章生成平台已启动：\n'
printf '  Web:    %s\n' "$WEB_URL"
printf '  API:    %s/api/v1/health\n' "$API_URL"
printf '  Worker: %s/health\n' "$WORKER_URL"
printf '  日志:   %s\n' "$RUNTIME_DIR"
printf '按 Ctrl+C 停止全部服务。\n\n'

if [ "${OPEN_BROWSER:-false}" = true ]; then
  if command -v open >/dev/null 2>&1; then
    open "$WEB_URL"
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$WEB_URL" >/dev/null 2>&1 || true
  fi
fi

while :; do
  for service in "API:$API_PID:$RUNTIME_DIR/api.log" "Worker:$WORKER_PID:$RUNTIME_DIR/worker.log" "Web:$WEB_PID:$RUNTIME_DIR/web.log"; do
    name=${service%%:*}
    remainder=${service#*:}
    pid=${remainder%%:*}
    log_file=${remainder#*:}
    if ! kill -0 "$pid" 2>/dev/null; then
      show_failure "$name" "$log_file"
    fi
  done
  sleep 1
done
