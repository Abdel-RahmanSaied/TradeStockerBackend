#!/bin/sh
# End-to-end check of the development stack (S01-T02) on a new, disposable
# Compose project, tradestocker-dev-smoke<random>, with its own volumes,
# secrets and ports (API 18380, frontend 15273 by default):
#   1. clean start with nothing loaded;
#   2. the real frontend browser tests against that stack's API;
#   3. up seeds only what is still missing (the browser tests made the rest);
#   4. a second up changes nothing;
#   5. a real down and up (new containers) keeps every byte of data.
# It never touches the normal app, the default tradestocker-dev project or any
# other project. Nothing is removed: the smoke project stays, and the last line
# prints the separate command that would remove its volumes.
#   scripts/dev-stack-smoke.sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
STACK="$ROOT/scripts/dev-stack.sh"
FRONTEND="${TS_FRONTEND_ROOT:-$(dirname "$ROOT")/TradeStockerFrontend}"
[ -f "$FRONTEND/package-lock.json" ] || { echo "smoke: sibling frontend not found; set TS_FRONTEND_ROOT" >&2; exit 1; }
TS_DEV_PROJECT="tradestocker-dev-smoke$(python3 -I -c 'import secrets; print(secrets.token_hex(4))')"
TS_DEV_API_PORT="${TS_SMOKE_API_PORT:-18380}"
TS_DEV_FRONTEND_PORT="${TS_SMOKE_FRONTEND_PORT:-15273}"
export TS_DEV_PROJECT TS_DEV_API_PORT TS_DEV_FRONTEND_PORT
SECRETS="$ROOT/.local/dev-stack/$TS_DEV_PROJECT"
OUT="$ROOT/.local/dev-stack-smoke/$TS_DEV_PROJECT"
mkdir -p "$OUT"
. "$ROOT/scripts/disposable-api.sh"
disposable_api_preflight "$TS_DEV_FRONTEND_PORT"
step() { echo "== smoke $TS_DEV_PROJECT: $*" >&2; }
fail() { echo "smoke FAILED: $*" >&2; exit 1; }
# summary_is FILE PYTHON-CONDITION: the condition, on the JSON summary s, must hold.
summary_is() {
  python3 -I -c "import json,sys; s=json.load(open(sys.argv[1])); sys.exit(0 if ($2) else 1)" "$1" || fail "$1 does not satisfy: $2 ($(cat "$1"))"
}
# Read-only compose calls for this smoke project only (same variables dev-stack.sh sets).
compose_smoke() {
  TS_DEV_SECRETS="$SECRETS" docker compose --project-name "$TS_DEV_PROJECT" --file "$ROOT/deploy/compose.yaml" "$@"
}
container() { compose_smoke ps --quiet platform-core; }

step "1. clean start, nothing loaded"
"$STACK" up --no-seed > "$OUT/summary-1.json"
summary_is "$OUT/summary-1.json" 's["status"] == "ok" and s["mode"] == "PAPER" and s["live_advisory_enabled"] is False and s["research_configured"] is False and s["accounts"] == [] and s["quotes"] == 0'

step "2. real frontend browser tests against this stack"
(cd "$FRONTEND" && TS_LOCAL_DEV_TOKEN="$(cat "$SECRETS/api_token")" FRONTEND_PORT="$TS_DEV_FRONTEND_PORT" \
  API_PROXY_TARGET="http://127.0.0.1:$TS_DEV_API_PORT" TS_E2E_REAL_SEC=0 CI=1 npm run test:e2e) > "$OUT/browser.log" 2>&1 || fail "browser tests; see $OUT/browser.log"

step "3. up seeds only what is missing"
"$STACK" up > "$OUT/summary-3.json"
summary_is "$OUT/summary-3.json" 's["live_advisory_enabled"] is False and [c for c, _ in s["accounts"]] == ["EGP", "USD"] and s["quotes"] > 0 and s["quote_modes"] == ["SYNTHETIC"]'
# A due paper expiry is a legitimate write on read; steps 3-5 must end before
# the earliest one, so require 10 minutes of margin rather than guess later.
summary_is "$OUT/summary-3.json" 's["next_expiry"] is None or __import__("datetime").datetime.fromisoformat(s["next_expiry"].replace("Z", "+00:00")) > __import__("datetime").datetime.now(__import__("datetime").timezone.utc) + __import__("datetime").timedelta(minutes=10)'
compose_smoke run --rm --no-deps research-worker cat /results/results.jsonl > "$OUT/results.jsonl"
cmp "$OUT/results.jsonl" "$ROOT/fixtures/synthetic/research/expected-results.jsonl" || fail "stored research results differ from the labelled fixture results"
"$STACK" digest > "$OUT/digest-3.txt"

step "4. a second up changes nothing"
"$STACK" up > "$OUT/summary-4.json"
"$STACK" digest > "$OUT/digest-4.txt"
cmp "$OUT/digest-3.txt" "$OUT/digest-4.txt" || fail "a second up changed data"

step "5. real down and up keeps every byte"
before=$(container)
"$STACK" down
"$STACK" up > "$OUT/summary-5.json"
after=$(container)
[ -n "$before" ] && [ -n "$after" ] && [ "$before" != "$after" ] || fail "platform-core container was not recreated"
"$STACK" digest > "$OUT/digest-5.txt"
cmp "$OUT/digest-3.txt" "$OUT/digest-5.txt" || fail "data changed across down and up"
cmp "$OUT/summary-3.json" "$OUT/summary-5.json" || fail "summary changed across down and up"

"$STACK" down
step "PASS. Evidence in $OUT. The project's volumes are kept; to remove them:"
echo "  TS_DEV_PROJECT=$TS_DEV_PROJECT TS_DEV_CONFIRM=$TS_DEV_PROJECT scripts/dev-stack.sh destroy-volumes" >&2
