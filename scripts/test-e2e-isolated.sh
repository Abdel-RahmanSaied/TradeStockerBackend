#!/bin/sh
# Browser E2E: the frontend's Playwright tests against a real platform-core
# that this script owns, kept apart from the normal app.
#
# Owned by this run (and nothing else):
#   - a new output folder .local/t05-e2e.XXXXXX (mktemp -d) holding the
#     platform-core binary built for this run, its log and Playwright's
#     artifacts (in the new child folder playwright/, which Playwright empties
#     before writing; nothing else is cleaned and nothing is deleted after);
#   - one new PostgreSQL container from the pinned CI image, labelled with
#     this run's ID, data in memory (tmpfs), on a random 127.0.0.1 port;
#   - one platform-core process from that binary on 127.0.0.1:18090, with a
#     fresh per-run token, no SEC contact and no NATS;
#   - the Vite server Playwright itself starts on 127.0.0.1:15173 and stops.
# The normal database (55432), bin/, .env.local and the normal API and
# dashboard (8080/5173) are never used.
#
# Teardown signals only the API process this script started, and only while
# it still runs this run's binary; then it stops only its own container after
# checking the exact ID and run label (--rm removes it).
set -eu
cd "$(dirname "$0")/.."
TS_REPO_ROOT=$(pwd -P)
TS_FRONTEND_ROOT="$(dirname "$TS_REPO_ROOT")/TradeStockerFrontend"
IMAGE=postgres:16.15-alpine@sha256:f4610795587492cd4fb887c2dc3de301b813278bb6f616992061141b5f15ac36
API_PORT=18090
FRONTEND_PORT=15173

# Real folders only: no symbolic link for the output parent or the frontend.
for dir in "$TS_REPO_ROOT/.local" "$TS_FRONTEND_ROOT"; do
  if [ -L "$dir" ] || [ ! -d "$dir" ]; then
    echo "$dir must be a real folder, not a symbolic link" >&2
    exit 1
  fi
done
# The frontend must be a clean checkout, so the run names an exact commit.
if ! git -C "$TS_FRONTEND_ROOT" diff --quiet HEAD --; then
  echo "the frontend has uncommitted changes; commit or stash them first" >&2
  exit 1
fi
FRONTEND_COMMIT=$(git -C "$TS_FRONTEND_ROOT" rev-parse HEAD)

TS_PYTHON=$(./scripts/toolchain.sh python)
export TS_PYTHON
. ./scripts/disposable-api.sh
# Refuse to start if either private port is in use.
disposable_api_preflight "$API_PORT" "$FRONTEND_PORT"
disposable_api_fresh_token

OUT=$(mktemp -d "$TS_REPO_ROOT/.local/t05-e2e.XXXXXX")
if [ -L "$OUT" ] || [ ! -d "$OUT" ]; then
  echo "output folder was not created as a real folder" >&2
  exit 1
fi
BIN="$OUT/platform-core"
GOTOOLCHAIN="go$(awk '$1 == "go" {print $2}' go.mod)"
export GOTOOLCHAIN
env -u GOFLAGS go build -o "$BIN" ./cmd/platform-core

RUN_ID=$(od -An -tx1 -N6 /dev/urandom | tr -d ' \n')
PASSWORD=$(od -An -tx1 -N16 /dev/urandom | tr -d ' \n')
NAME="tradestocker-t05-e2e-pg-$RUN_ID"
DB="tradestocker_test_$RUN_ID"
ID=$(docker run --detach --rm --name "$NAME" \
  --label tradestocker.purpose=t05-isolated-e2e --label "tradestocker.run=$RUN_ID" \
  --publish 127.0.0.1::5432 --tmpfs /var/lib/postgresql/data \
  --env POSTGRES_USER=paper_test --env "POSTGRES_PASSWORD=$PASSWORD" --env "POSTGRES_DB=$DB" \
  "$IMAGE")
case "$ID" in
  *[!0-9a-f]* | "") echo "unexpected container id from docker run" >&2; exit 1 ;;
esac

SERVER_PID=''
cleanup() {
  if [ -n "$SERVER_PID" ]; then
    command=$(ps -o command= -p "$SERVER_PID" 2>/dev/null || true)
    case "$command" in
      "$BIN"*)
        kill -TERM "$SERVER_PID"
        wait "$SERVER_PID" 2>/dev/null || true
        echo "stopped test API $SERVER_PID" >&2 ;;
      *) echo "test API $SERVER_PID not running this run's binary; nothing signalled" >&2 ;;
    esac
  fi
  label=$(docker inspect --format '{{index .Config.Labels "tradestocker.run"}}' "$ID" 2>/dev/null || true)
  if [ "$label" = "$RUN_ID" ]; then
    docker stop --time 5 "$ID" >/dev/null
    echo "stopped test container $NAME" >&2
  else
    echo "test container $ID not found or not this run's; nothing stopped" >&2
  fi
  echo "outputs kept in $OUT" >&2
}
trap cleanup EXIT
trap 'exit 130' INT TERM

PORT=$(docker port "$ID" 5432/tcp | sed -n 's/^127\.0\.0\.1:\([0-9][0-9]*\)$/\1/p' | head -n 1)
if [ -z "$PORT" ] || [ "$PORT" = 55432 ]; then
  echo "no private local port for the test container" >&2
  exit 1
fi
i=0
until docker exec "$ID" pg_isready -q -h 127.0.0.1 -U paper_test -d "$DB"; do
  i=$((i + 1))
  if [ "$i" -ge 60 ]; then
    echo "test PostgreSQL did not start" >&2
    exit 1
  fi
  sleep 1
done
echo "isolated PostgreSQL $NAME ($ID) on 127.0.0.1:$PORT; API 127.0.0.1:$API_PORT; dashboard 127.0.0.1:$FRONTEND_PORT; frontend $FRONTEND_COMMIT; outputs $OUT" >&2

env -u DATABASE_URL_FILE -u TS_LOCAL_DEV_TOKEN_FILE -u TS_CONTAINER_LOCAL -u TS_NATS_URL -u TS_RESEARCH_WORKERS \
  DATABASE_URL="postgres://paper_test:$PASSWORD@127.0.0.1:$PORT/$DB?sslmode=disable" \
  TS_BIND="127.0.0.1:$API_PORT" TS_DASHBOARD_ORIGIN="http://127.0.0.1:$FRONTEND_PORT" \
  TS_SEC_USER_AGENT='' TS_MODE=PAPER \
  "$BIN" >"$OUT/server.log" 2>&1 &
SERVER_PID=$!
disposable_api_wait "$API_PORT" "$SERVER_PID" || { echo "See $OUT/server.log" >&2; exit 1; }

# FRONTEND_URL would point Playwright away from its own Vite server and
# SCREENSHOT_PATH would write outside the run folder, so neither is inherited.
cd "$TS_FRONTEND_ROOT"
env -u FRONTEND_URL -u SCREENSHOT_PATH \
  FRONTEND_PORT="$FRONTEND_PORT" TS_E2E_REAL_SEC=0 API_PROXY_TARGET="http://127.0.0.1:$API_PORT" CI=1 \
  npm run test:e2e -- --output "$OUT/playwright"
