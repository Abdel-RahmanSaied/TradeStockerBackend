#!/bin/sh
set -eu
cd "$(dirname "$0")/.."
set -a
. ./.env.local
set +a
TS_TEST_DB="tradestocker_test_$(python3 -c 'import secrets;print(secrets.token_hex(6))')"
export TS_TEST_DB
cleanup() { docker compose --env-file .env.local exec -T postgres dropdb -U tradestocker --force "$TS_TEST_DB" >/dev/null; }
docker compose --env-file .env.local exec -T postgres createdb -U tradestocker "$TS_TEST_DB"
trap cleanup EXIT INT TERM
TEST_DATABASE_URL="postgres://tradestocker:${TS_DB_PASSWORD}@127.0.0.1:55432/${TS_TEST_DB}?sslmode=disable" go test -race -v ./internal/store ./internal/research
