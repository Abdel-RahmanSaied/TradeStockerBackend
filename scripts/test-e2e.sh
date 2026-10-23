#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
TS_REPO_ROOT=$(pwd)
TS_FRONTEND_ROOT="${TS_FRONTEND_ROOT:-$(dirname "$TS_REPO_ROOT")/TradeStockerFrontend}"
set -a
. ./.env.local
set +a
. ./scripts/disposable-api.sh
# Ports are configurable so a run can stay isolated from a normal app on 8080/5173.
TS_E2E_API_PORT="${TS_E2E_API_PORT:-18080}"
TS_E2E_FRONTEND_PORT="${TS_E2E_FRONTEND_PORT:-5173}"
# Refuse to start if the disposable API port or the dashboard port is in use.
disposable_api_preflight "$TS_E2E_API_PORT" "$TS_E2E_FRONTEND_PORT"
# A per-run token: only this run's API accepts it; .env.local is not changed.
disposable_api_fresh_token
# SEC research is off by default: the disposable API gets no SEC contact and makes
# no external request. TS_E2E_REAL_SEC=1 uses the approved TS_SEC_USER_AGENT from
# .env.local and contacts data.sec.gov (about four requests).
TS_E2E_REAL_SEC="${TS_E2E_REAL_SEC:-0}"
E2E_SEC_AGENT=''
if [ "$TS_E2E_REAL_SEC" = "1" ]; then
  [ -n "${TS_SEC_USER_AGENT:-}" ] || { printf 'TS_E2E_REAL_SEC=1 needs TS_SEC_USER_AGENT in .env.local\n' >&2; exit 1; }
  E2E_SEC_AGENT="$TS_SEC_USER_AGENT"
fi
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
DATABASE_URL="postgres://tradestocker:${TS_DB_PASSWORD}@127.0.0.1:55432/${TS_TEST_DB}?sslmode=disable" TS_BIND="127.0.0.1:$TS_E2E_API_PORT" TS_DASHBOARD_ORIGIN="http://127.0.0.1:$TS_E2E_FRONTEND_PORT" TS_SEC_USER_AGENT="$E2E_SEC_AGENT" ./bin/platform-core >.local/e2e-server.log 2>&1 &
TS_SERVER_PID=$!
disposable_api_wait "$TS_E2E_API_PORT" "$TS_SERVER_PID" || { printf 'See .local/e2e-server.log\n' >&2; exit 1; }
cd "$TS_FRONTEND_ROOT"
FRONTEND_PORT="$TS_E2E_FRONTEND_PORT" TS_E2E_REAL_SEC="$TS_E2E_REAL_SEC" API_PROXY_TARGET="http://127.0.0.1:$TS_E2E_API_PORT" CI=1 npm run test:e2e
