#!/bin/sh
# Fixed-history before/after benchmark of paper reads (S00-T09). Builds the API at
# a base Git ref ("before") and from the working tree ("after"), runs each against
# its own fresh database in the spike's isolated PostgreSQL (port 55532) with the
# API on 18180, and prints both results. It never uses the normal database
# (55432), API (8080), dashboard (5173) or the .env.local token.
#   scripts/paper-read-bench.sh <base-git-ref>
set -eu
cd "$(dirname "$0")/.."
ROOT=$(pwd)
BASE_REF="${1:?usage: scripts/paper-read-bench.sh <base-git-ref>}"
WORK="$ROOT/.local/paper-read-bench"
. ./scripts/disposable-api.sh
disposable_api_preflight 55532 18180
rm -rf "$WORK"
mkdir -p "$WORK/base"
git archive "$BASE_REF" | tar -x -C "$WORK/base"
(cd "$WORK/base" && go build -o "$WORK/platform-core-before" ./cmd/platform-core)
go build -o "$WORK/platform-core-after" ./cmd/platform-core
go build -o "$WORK/paperbench" ./scripts/paperbench
SPIKE_DB_PASSWORD=$(python3 -c 'import secrets; print(secrets.token_hex(16))')
export SPIKE_DB_PASSWORD
COMPOSE="docker compose -f $ROOT/spikes/s00-t05/compose.yaml"
API_PID=''
cleanup() {
  if [ -n "$API_PID" ]; then kill "$API_PID" 2>/dev/null || true; wait "$API_PID" 2>/dev/null || true; fi
  $COMPOSE down -v >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM
$COMPOSE up -d --wait postgres >/dev/null

for label in before after; do
  printf '%s: seeding history and measuring reads\n' "$label" >&2
  $COMPOSE exec -T postgres createdb -U spike "bench_$label"
  DB="postgres://spike:${SPIKE_DB_PASSWORD}@127.0.0.1:55532/bench_$label?sslmode=disable"
  disposable_api_fresh_token
  DATABASE_URL="$DB" TS_BIND=127.0.0.1:18180 TS_SEC_USER_AGENT='' "$WORK/platform-core-$label" >"$WORK/api-$label.log" 2>&1 &
  API_PID=$!
  disposable_api_wait 18180 "$API_PID" || { printf 'See %s/api-%s.log\n' "$WORK" "$label" >&2; exit 1; }
  BENCH_LABEL="$label ($([ "$label" = before ] && echo "$BASE_REF" || echo working-tree))" BENCH_API=http://127.0.0.1:18180/api/v1 BENCH_DB_URL="$DB" \
    "$WORK/paperbench" >"$WORK/result-$label.json"
  kill "$API_PID"
  wait "$API_PID" 2>/dev/null || true
  API_PID=''
done

python3 -I -B - "$WORK" <<'PY'
import json, sys
from pathlib import Path
work = Path(sys.argv[1])
before, after = (json.loads((work / f"result-{n}.json").read_text()) for n in ("before", "after"))
same = all(before[k] == after[k] for k in ("history_evaluations", "recommendations", "audit_events"))
print(json.dumps({"same_history": same, "before": before, "after": after,
                  "dashboard_reads_per_second_ratio": round(after["dashboard_reads"]["per_second"] / before["dashboard_reads"]["per_second"], 2),
                  "dashboard_p50_ratio": round(after["dashboard_reads"]["p50_ms"] / before["dashboard_reads"]["p50_ms"], 3)}, indent=1))
sys.exit(0 if same else 1)
PY
