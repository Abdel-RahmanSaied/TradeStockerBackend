#!/bin/sh
# Local-only combined launcher. Ctrl-C stops app processes and preserves PostgreSQL data.
set -eu
cd "$(dirname "$0")/.."
TS_REPO_ROOT=$(pwd)
TS_FRONTEND_ROOT="${TS_FRONTEND_ROOT:-$(dirname "$TS_REPO_ROOT")/TradeStockerFrontend}"
[ -f "$TS_FRONTEND_ROOT/package-lock.json" ] || { printf 'Sibling frontend not found. Set TS_FRONTEND_ROOT.\n' >&2; exit 1; }
# Port check only: it connects, so a live server is refused but leftover TIME_WAIT
# sockets from a finished run are not. The normal app keeps its .env.local token.
. ./scripts/disposable-api.sh
disposable_api_preflight 8080 5173
./scripts/setup-local.sh
set -a
. ./.env.local
set +a
mkdir -p bin .local
go build -o bin/platform-core ./cmd/platform-core
(cd "$TS_FRONTEND_ROOT" && npm ci)
TS_API_PID=''; TS_FRONT_PID=''
cleanup() {
  trap - EXIT INT TERM
  [ -z "$TS_API_PID" ] || kill "$TS_API_PID" 2>/dev/null || true
  [ -z "$TS_FRONT_PID" ] || kill "$TS_FRONT_PID" 2>/dev/null || true
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM
./bin/platform-core >.local/api.log 2>&1 &
TS_API_PID=$!
python3 -B - <<'PY'
import time,urllib.request
for _ in range(100):
    try:
        with urllib.request.urlopen('http://127.0.0.1:8080/api/v1/health',timeout=1) as r:
            if r.status==200: break
    except OSError: time.sleep(.1)
else: raise SystemExit('API not ready; inspect .local/api.log')
PY
(cd "$TS_FRONTEND_ROOT" && exec node node_modules/vite/bin/vite.js --host 127.0.0.1 --port 5173 --strictPort) >.local/frontend.log 2>&1 &
TS_FRONT_PID=$!
printf '\nTradeStocker: http://127.0.0.1:5173\nUse Access and the TS_LOCAL_DEV_TOKEN value from backend/.env.local.\nOptional demo data: scripts/seed-paper.sh in a second terminal.\nApp logs: .local/api.log and .local/frontend.log\nCtrl-C stops the app; database data is preserved.\n'
while kill -0 "$TS_API_PID" 2>/dev/null && kill -0 "$TS_FRONT_PID" 2>/dev/null; do sleep 1; done
printf 'One local app process stopped. Inspect .local/ logs.\n' >&2
exit 1
