#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if [ ! -f .env.local ]; then
  printf 'Run scripts/setup-local.sh first.\n' >&2
  exit 1
fi
set -a
. ./.env.local
set +a
exec go run ./cmd/platform-core
