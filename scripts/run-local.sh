#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if [ ! -f .env.local ]; then
  printf 'Run scripts/setup-local.sh first.\n' >&2
  exit 1
fi
# The pinned research Python (platform-core refuses any other). A plain
# assignment, never eval, so a missing toolchain stops this script.
TS_PYTHON=$(./scripts/toolchain.sh python)
export TS_PYTHON
set -a
. ./.env.local
set +a
exec go run ./cmd/platform-core
