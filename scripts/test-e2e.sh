#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
TS_REPO_ROOT=$(pwd)
TS_FRONTEND_ROOT="${TS_FRONTEND_ROOT:-$(dirname "$TS_REPO_ROOT")/TradeStockerFrontend}"
set -a
. ./.env.local
set +a
TS_TEST_DB="tradestocker_test_$(python3 -c 'import secrets;print(secrets.token_hex(6))')"
TS_SERVER_PID=''
cleanup() {
  cd "$TS_REPO_ROOT"
  if [ -n "$TS_SERVER_PID" ]; then kill "$TS_SERVER_PID" 2>/dev/null || true; wait "$TS_SERVER_PID" 2>/dev/null || true; fi
  docker compose --env-file .env.local exec -T postgres dropdb -U tradestocker --force "$TS_TEST_DB" >/dev/null
}
docker compose --env-file .env.local exec -T postgres createdb -U tradestocker "$TS_TEST_DB"
trap cleanup EXIT INT TERM
mkdir -p bin .local
go build -o bin/platform-core ./cmd/platform-core
DATABASE_URL="postgres://tradestocker:${TS_DB_PASSWORD}@127.0.0.1:55432/${TS_TEST_DB}?sslmode=disable" TS_BIND=127.0.0.1:18080 ./bin/platform-core >.local/e2e-server.log 2>&1 &
TS_SERVER_PID=$!
python3 -B - <<'PY'
import time,urllib.request
for _ in range(100):
    try:
        with urllib.request.urlopen('http://127.0.0.1:18080/api/v1/health',timeout=1) as r:
            if r.status==200: break
    except OSError: time.sleep(.1)
else: raise SystemExit('Disposable E2E API did not start; inspect .local/e2e-server.log')
PY
cd "$TS_FRONTEND_ROOT"
API_PROXY_TARGET=http://127.0.0.1:18080 CI=1 npm run test:e2e
