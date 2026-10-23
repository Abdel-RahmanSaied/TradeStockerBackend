#!/bin/sh
# Builds the three entrypoints into OUT and checks the result:
#   OUT/<goos>-<goarch>/platform-core, market-ingestion  static Go binaries
#   OUT/tradestocker_research-<v>-py3-none-any.whl     research-worker package
#   OUT/SHA256SUMS
# It never deletes anything: OUT must be a new path or an empty folder, and the
# repository, home and their parent folders are refused (scripts/package-out.sh).
# Go binaries use exactly the Go release on go.mod's go line (GOTOOLCHAIN set to
# it; go.mod alone is only a minimum), -trimpath and CGO off; the wheel uses uv's
# built-in backend offline. Binaries record the Git commit they came from. Paths
# pass by plain assignment only, never eval.
#   scripts/package.sh <new-or-empty-folder>
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
. "$ROOT/scripts/package-out.sh"
OUT=$(prepare_out "${1:?usage: scripts/package.sh <new-or-empty-folder>}" "$ROOT")
GOTOOLCHAIN="go$(awk '$1 == "go" {print $2}' "$ROOT/go.mod")"
export GOTOOLCHAIN
GOT=$(cd "$ROOT" && go env GOVERSION)
[ "$GOT" = "$GOTOOLCHAIN" ] || { echo "Go is $GOT, want $GOTOOLCHAIN" >&2; exit 1; }
"$ROOT/scripts/toolchain.sh"
. "$ROOT/scripts/toolchain-paths.sh"
PY=$("$ROOT/scripts/toolchain.sh" python)
PLATFORM="$(go env GOOS)-$(go env GOARCH)"
mkdir "$OUT/$PLATFORM"
(cd "$ROOT" && CGO_ENABLED=0 go build -trimpath -o "$OUT/$PLATFORM/" ./cmd/platform-core ./cmd/market-ingestion)
"$UV" build --quiet --offline --project "$ROOT/research" --python "$PY" --wheel --out-dir "$OUT"
(cd "$OUT" && find . -type f ! -name SHA256SUMS ! -name .gitignore | LC_ALL=C sort | sed 's|^\./||' |
  while IFS= read -r file; do shasum -a 256 "$file"; done > SHA256SUMS)

# Smoke checks: each artifact starts and refuses unsafe or unknown input. The
# check folder is new and is left in place, not deleted.
WORK=$(mktemp -d "${TMPDIR:-/tmp}/tradestocker-package-check.XXXXXX")
fail() { echo "package check failed: $*" >&2; exit 1; }
(cd "$OUT" && shasum -a 256 -c SHA256SUMS >/dev/null) || fail "checksums"
out=$(env -i PATH=/usr/bin:/bin "$OUT/$PLATFORM/platform-core" 2>&1) && fail "platform-core started without configuration"
case "$out" in *TS_LOCAL_DEV_TOKEN*) ;; *) fail "platform-core: $out" ;; esac
out=$(env -i PATH=/usr/bin:/bin TS_MARKET_PROVIDER=alpaca "$OUT/$PLATFORM/market-ingestion" 2>&1) && fail "market-ingestion accepted a real feed"
case "$out" in *"not qualified"*) ;; *) fail "market-ingestion: $out" ;; esac
"$UV" venv --quiet --python "$PY" "$WORK/venv"
"$UV" pip install --quiet --offline --python "$WORK/venv/bin/python" "$OUT"/tradestocker_research-*.whl
"$WORK/venv/bin/python" -I -c 'import tradestocker_research.financials, tradestocker_research.worker' || fail "package import"
result=$(printf '%s\n' '{"task":"research-stub-v1","input":{"symbol":"AAPL","mode":"PAPER"}}' '{"task":"place-order","input":{}}' | "$WORK/venv/bin/research-worker")
[ "$(printf '%s\n' "$result" | sed -n 1p | "$PY" -I -c 'import json,sys; print(json.load(sys.stdin)["outcome"])')" = INSUFFICIENT_DATA ] || fail "research-worker stub: $result"
[ "$(printf '%s\n' "$result" | sed -n 2p | "$PY" -I -c 'import json,sys; print(json.load(sys.stdin)["status"])')" = REJECTED ] || fail "research-worker accepted an unknown task: $result"
echo "packaged and checked: $OUT (check folder kept: $WORK)" >&2
