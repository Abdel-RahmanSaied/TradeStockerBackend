#!/bin/sh
# Local development stack (S01-T02): PostgreSQL, NATS and the three entrypoints
# in their own Compose project (deploy/compose.yaml). It never uses the normal
# tradestocker-local project, its ports, its volumes or .env.local.
#
#   scripts/dev-stack.sh up [--no-seed]  build, start and wait until healthy (progress
#       on stderr; stdout is only the final JSON summary). Seeds
#       the synthetic fixtures only where the workspace has none yet; anything
#       already there is left as it is, so a restart changes no data.
#       --no-seed starts without loading anything (for browser tests that need
#       an empty workspace).
#   scripts/dev-stack.sh refresh-fixtures  CHANGES DATA: reloads the synthetic
#       market observations (new timestamps and one audit event) and rewrites
#       the research fixture results.
#   scripts/dev-stack.sh status | logs | digest
#   scripts/dev-stack.sh stop  stops the containers; keeps everything
#   scripts/dev-stack.sh down  removes containers and networks; keeps volumes,
#       images and secrets
#   scripts/dev-stack.sh destroy-volumes  REMOVES this project's volumes. Only for
#       a tradestocker-dev-<suffix> project, and only with TS_DEV_CONFIRM set to
#       that project name. The default project's volumes are never removed.
#
# Settings: the environment, then config/dev.env (ignored by Git), then the
# defaults (see config/example.env). Values are read as plain text, never run.
# Secrets are created once per project in .local/dev-stack/<project>/ (folder
# 700, files 600), are never overwritten and never printed.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
NL='
'
RESERVED="80 443 4222 5173 5432 8080 8222 15173 18080 18090 18180 54222 55432 55532 58222"
fail() { echo "dev-stack: $*" >&2; exit 1; }

# setting NAME DEFAULT: the environment value, else the last NAME= line of
# config/dev.env, else DEFAULT. NAME is always one of the fixed names below.
setting() {
  if value=$(printenv "$1"); then
    printf '%s' "$value"
    return
  fi
  if [ -f "$ROOT/config/dev.env" ]; then
    value=$(sed -n "s/^$1=//p" "$ROOT/config/dev.env" | tail -n 1)
    if [ -n "$value" ]; then
      printf '%s' "$value"
      return
    fi
  fi
  printf '%s' "$2"
}

port_ok() {
  printf '%s\n' "$1" | grep -Eqx '[1-9][0-9]{3,4}' || return 1
  [ "$1" -ge 1024 ] && [ "$1" -le 65535 ] || return 1
  case " $RESERVED " in *" $1 "*) return 1 ;; esac
}

PROJECT=$(setting TS_DEV_PROJECT tradestocker-dev)
API_PORT=$(setting TS_DEV_API_PORT 18280)
FRONTEND_PORT=$(setting TS_DEV_FRONTEND_PORT 15273)
case "$PROJECT$API_PORT$FRONTEND_PORT" in *"$NL"*) fail "settings must be a single line" ;; esac
if ! printf '%s\n' "$PROJECT" | grep -Eqx 'tradestocker-dev(-[a-z0-9]+)*' || [ ${#PROJECT} -gt 48 ]; then
  fail "TS_DEV_PROJECT must be tradestocker-dev or tradestocker-dev-<suffix> (lowercase letters, digits, dashes)"
fi
port_ok "$API_PORT" || fail "TS_DEV_API_PORT must be 1024-65535 and not a reserved port ($RESERVED)"
port_ok "$FRONTEND_PORT" || fail "TS_DEV_FRONTEND_PORT must be 1024-65535 and not a reserved port ($RESERVED)"
[ "$API_PORT" != "$FRONTEND_PORT" ] || fail "the API and frontend ports must differ"
SECRETS="$ROOT/.local/dev-stack/$PROJECT"
API="http://127.0.0.1:$API_PORT"
. "$ROOT/scripts/disposable-api.sh"

compose() {
  TS_DEV_PROJECT="$PROJECT" TS_DEV_API_PORT="$API_PORT" TS_DEV_FRONTEND_PORT="$FRONTEND_PORT" TS_DEV_SECRETS="$SECRETS" \
    docker compose --project-name "$PROJECT" --file "$ROOT/deploy/compose.yaml" "$@"
}

api() {
  api_command=$1
  shift
  python3 -I "$ROOT/scripts/dev_stack_api.py" "$api_command" --api "$API" --token-file "$SECRETS/api_token" "$@"
}

running() {
  [ -n "$(compose ps --status running --quiet platform-core)" ]
}

ensure_secrets() {
  if [ -e "$SECRETS" ] || [ -L "$SECRETS" ]; then
    [ -d "$SECRETS" ] && [ ! -L "$SECRETS" ] || fail "$SECRETS is not a plain folder"
    [ -n "$(find "$SECRETS" -prune -perm 0700)" ] || fail "$SECRETS must have mode 700"
    for name in postgres_password api_token database_url; do
      [ -f "$SECRETS/$name" ] && [ ! -L "$SECRETS/$name" ] || fail "secret $name is missing in $SECRETS; use a new project name instead of guessing"
      [ -n "$(find "$SECRETS/$name" -prune -perm 0600)" ] || fail "$SECRETS/$name must have mode 600"
    done
    return
  fi
  (umask 077 && mkdir -p "$SECRETS" && python3 -I - "$SECRETS" <<'PY'
import os, secrets, sys
folder = sys.argv[1]
password = secrets.token_hex(24)
values = {
    "postgres_password": password,
    "api_token": secrets.token_hex(32),
    "database_url": "postgres://tradestocker_dev:%s@dev-postgres:5432/tradestocker_dev?sslmode=disable" % password,
}
for name, value in values.items():
    fd = os.open(os.path.join(folder, name), os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, "w") as f:
        f.write(value)
PY
  )
  echo "dev-stack: created fresh secrets in $SECRETS (not printed)" >&2
}

# seed: only the parts a new workspace is missing; existing data is untouched.
seed() {
  if api has-quotes; then
    echo "dev-stack: market observations present; left as they are" >&2
  else
    status=$?
    [ "$status" = 3 ] || fail "API check failed"
    compose run --rm market-ingestion >&2
  fi
  api seed-accounts --fixture "$ROOT/fixtures/synthetic/paper-accounts.json" >&2
  if compose run --rm --no-deps research-worker test -s /results/results.jsonl >&2; then
    echo "dev-stack: research fixture results present; left as they are" >&2
  else
    compose run --rm --no-deps research-worker >&2
  fi
}

up() {
  load=1
  case "$#:${1:-}" in
    0:) ;;
    1:--no-seed) load=0 ;;
    *) fail "usage: scripts/dev-stack.sh up [--no-seed]" ;;
  esac
  ensure_secrets
  if ! running; then
    disposable_api_preflight "$API_PORT"
  fi
  compose build platform-core >&2
  compose run --rm --no-deps dev-nats-volume >&2
  compose up --detach --wait dev-postgres dev-nats platform-core >&2
  if [ "$load" = 1 ]; then
    seed
  fi
  api summary
  echo "dev-stack: $PROJECT is ready at $API" >&2
}

digest() {
  running || fail "$PROJECT is not running"
  printf 'paper '
  compose exec -T dev-postgres psql -U tradestocker_dev -d tradestocker_dev -At -v ON_ERROR_STOP=1 \
    -c "SET default_transaction_read_only = on" -c "SELECT md5(document::text) FROM paper_workspace WHERE id = 1" | grep -v '^SET$'
  printf 'research '
  compose run --rm --no-deps research-worker sha256sum /results/results.jsonl
}

command=${1:-}
[ $# -gt 0 ] && shift
case "$command" in
  up) up "$@" ;;
  refresh-fixtures)
    [ $# -eq 0 ] || fail "refresh-fixtures takes no arguments"
    running || fail "$PROJECT is not running; run up first"
    echo "dev-stack: refresh-fixtures CHANGES DATA in $PROJECT (new observation timestamps, an audit event, rewritten research results)" >&2
    compose run --rm market-ingestion >&2
    compose run --rm --no-deps research-worker >&2
    ;;
  status)
    [ $# -eq 0 ] || fail "status takes no arguments"
    compose ps
    if running; then api summary; fi
    ;;
  logs)
    [ $# -eq 0 ] || fail "logs takes no arguments"
    compose logs --no-color --tail 200
    ;;
  digest)
    [ $# -eq 0 ] || fail "digest takes no arguments"
    digest
    ;;
  stop)
    [ $# -eq 0 ] || fail "stop takes no arguments"
    compose stop
    ;;
  down)
    [ $# -eq 0 ] || fail "down takes no arguments"
    compose down
    ;;
  destroy-volumes)
    [ $# -eq 0 ] || fail "destroy-volumes takes no arguments"
    [ "$PROJECT" != tradestocker-dev ] || fail "the default project's volumes are never removed here; use a tradestocker-dev-<suffix> project for throwaway stacks"
    [ "${TS_DEV_CONFIRM:-}" = "$PROJECT" ] || fail "set TS_DEV_CONFIRM=$PROJECT to remove the volumes of $PROJECT"
    echo "dev-stack: REMOVING the containers, networks and volumes of $PROJECT" >&2
    compose down --volumes
    ;;
  *) fail "usage: scripts/dev-stack.sh up [--no-seed] | refresh-fixtures | status | logs | digest | stop | down | destroy-volumes" ;;
esac
