#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
if [ ! -f .env.local ]; then
  umask 077
  python3 -B - <<'PY'
import secrets
from pathlib import Path
password=secrets.token_hex(24)
token=secrets.token_hex(32)
Path('.env.local').write_text('TS_DB_PASSWORD='+password+'\nTS_LOCAL_DEV_TOKEN='+token+'\nDATABASE_URL=postgres://tradestocker:'+password+'@127.0.0.1:55432/tradestocker_paper?sslmode=disable\nTS_MODE=PAPER\n')
PY
fi
chmod 600 .env.local
./scripts/toolchain.sh
docker compose --env-file .env.local up -d --wait postgres
printf 'Local database ready. Run scripts/run-local.sh, then start the frontend. API token is in .env.local (keep private).\n'
