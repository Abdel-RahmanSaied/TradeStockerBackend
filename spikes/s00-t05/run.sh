#!/bin/sh
# S00-T05 compatibility spike. Everything stays inside spikes/s00-t05: tools and
# caches in .tools/, results in .local/, and a separate Compose project whose data
# lives in memory. It never uses the normal database (55432), API (8080),
# dashboard (5173) or .env.local token. Set SPIKE_SAMPLE_DIR to a folder with
# submissions.json and companyfacts.json to also measure source storage size.
set -eu
cd "$(dirname "$0")"
SPIKE=$(pwd)
ROOT=$(cd ../.. && pwd)
T="$SPIKE/.tools"
OUT="$SPIKE/.local"
mkdir -p "$T/bin" "$OUT"
. "$ROOT/scripts/disposable-api.sh"
disposable_api_preflight 55532 54222 58222 18180

export GOMODCACHE="$T/gomodcache" GOCACHE="$T/gocache" GOPATH="$T/gopath"
# Not UV_NO_CONFIG: it would also ignore python/pyproject.toml [tool.uv], including
# the exclude-newer supply-chain cutoff.
export UV_CACHE_DIR="$T/uv-cache" UV_PYTHON_INSTALL_DIR="$T/python" UV_PYTHON_BIN_DIR="$T/pybin" \
  UV_PROJECT_ENVIRONMENT="$T/venv" UV_PYTHON_PREFERENCE=only-managed
SPIKE_DB_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_hex(16))')
export SPIKE_DB_PASSWORD
export SPIKE_PG_URL="postgres://spike:${SPIKE_DB_PASSWORD}@127.0.0.1:55532/spike?sslmode=disable"
export SPIKE_NATS_URL="nats://127.0.0.1:54222"
COMPOSE="docker compose -f $SPIKE/compose.yaml"
API_PID=''
cleanup() {
  if [ -n "$API_PID" ]; then kill "$API_PID" 2>/dev/null || true; wait "$API_PID" 2>/dev/null || true; fi
  $COMPOSE down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

echo "1/5 tools and pinned dependencies (local to .tools)" >&2
if [ ! -x "$T/uv-venv/bin/uv" ]; then
  python3 -m venv "$T/uv-venv"
  "$T/uv-venv/bin/pip" install --quiet --disable-pip-version-check --no-cache-dir --require-hashes --only-binary :all: -r "$SPIKE/tools-requirements.txt"
fi
UV="$T/uv-venv/bin/uv"
# Exact interpreter: python/.python-version pins the patch release.
"$UV" python install "$(cat python/.python-version)" --no-bin >/dev/null 2>&1
(cd python && "$UV" sync --locked --quiet --python "$(cat .python-version)")
(cd go && go build -o "$T/bin/s00t05" . && go build -o "$T/bin/loadprofile" ./cmd/loadprofile)
(cd "$ROOT" && go build -o "$T/bin/platform-core" ./cmd/platform-core)
PY="$T/venv/bin/python"

echo "2/5 disposable PostgreSQL and NATS JetStream" >&2
$COMPOSE up -d --wait >/dev/null

echo "3/5 Go: outbox -> JetStream -> inbox" >&2
"$T/bin/s00t05" >"$OUT/go-jetstream.json"

echo "4/5 Python: JetStream, then checkpoint in one process and resume in another" >&2
"$PY" python/spike_checks.py nats >"$OUT/python-nats.json"
"$PY" python/spike_checks.py checkpoint start >"$OUT/python-checkpoint-start.json"
"$PY" python/spike_checks.py checkpoint resume >"$OUT/python-checkpoint-resume.json"

echo "5/5 product paper API load profile (own database, port 18180)" >&2
$COMPOSE exec -T postgres createdb -U spike paper_load
disposable_api_fresh_token
DATABASE_URL="postgres://spike:${SPIKE_DB_PASSWORD}@127.0.0.1:55532/paper_load?sslmode=disable" TS_BIND=127.0.0.1:18180 TS_SEC_USER_AGENT='' \
  "$T/bin/platform-core" >"$OUT/api.log" 2>&1 &
API_PID=$!
disposable_api_wait 18180 "$API_PID" || { printf 'See %s/api.log\n' "$OUT" >&2; exit 1; }
SPIKE_API=http://127.0.0.1:18180/api/v1 "$T/bin/loadprofile" >"$OUT/load-profile.json"
$COMPOSE exec -T postgres psql -U spike -d paper_load -Atc \
  "SELECT json_build_object('table_bytes', pg_total_relation_size('paper_workspace'), 'document_bytes', octet_length(document::text)) FROM paper_workspace" >"$OUT/paper-size.json"

echo "vulnerability check: OSV for every locked Python package" >&2
(cd python && "$PY" osv_check.py >"$OUT/osv.json")

"$PY" check_results.py "$OUT"
