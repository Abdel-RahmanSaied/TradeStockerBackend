#!/bin/sh
# Go tests that need PostgreSQL, run against a database this script owns: one
# new container from the pinned CI image, data in memory (tmpfs, no volume), on
# a random 127.0.0.1 port chosen by Docker. The normal local database (55432)
# and its Compose projects are never used.
#
# On exit the script stops only the container it started, after checking its
# exact ID and run label; --rm then removes that container. It deletes no
# files, volumes, images or databases and stops nothing else.
#
# Usage: scripts/test-isolated.sh [./pkg ...]   (TEST_RUN=<regexp> to filter)
# Packages run one at a time (-p 1), so no two packages share the database.
set -eu
cd "$(dirname "$0")/.."

IMAGE=postgres:16.15-alpine@sha256:f4610795587492cd4fb887c2dc3de301b813278bb6f616992061141b5f15ac36

for pkg in "$@"; do
  case "$pkg" in
    ./*) ;;
    *) echo "packages must start with ./ (got $pkg)" >&2; exit 2 ;;
  esac
done
[ "$#" -gt 0 ] || set -- ./internal/store ./internal/research ./internal/natsbus ./internal/httpapi ./cmd/platform-core

RUN_ID=$(od -An -tx1 -N6 /dev/urandom | tr -d ' \n')
PASSWORD=$(od -An -tx1 -N16 /dev/urandom | tr -d ' \n')
NAME="tradestocker-t05-test-pg-$RUN_ID"
DB="tradestocker_test_$RUN_ID"

ID=$(docker run --detach --rm --name "$NAME" \
  --label tradestocker.purpose=t05-isolated-test --label "tradestocker.run=$RUN_ID" \
  --publish 127.0.0.1::5432 --tmpfs /var/lib/postgresql/data \
  --env POSTGRES_USER=paper_test --env "POSTGRES_PASSWORD=$PASSWORD" --env "POSTGRES_DB=$DB" \
  "$IMAGE")
case "$ID" in
  *[!0-9a-f]* | "") echo "unexpected container id from docker run" >&2; exit 1 ;;
esac

stop_owned() {
  label=$(docker inspect --format '{{index .Config.Labels "tradestocker.run"}}' "$ID" 2>/dev/null || true)
  if [ "$label" = "$RUN_ID" ]; then
    docker stop --time 5 "$ID" >/dev/null
    echo "stopped test container $NAME" >&2
  else
    echo "test container $ID not found or not this run's; nothing stopped" >&2
  fi
}
trap stop_owned EXIT
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

GOTOOLCHAIN="go$(awk '$1 == "go" {print $2}' go.mod)"
TS_PYTHON=$(./scripts/toolchain.sh python)
TS_REQUIRE_PYTHON=1
TEST_DATABASE_URL="postgres://paper_test:$PASSWORD@127.0.0.1:$PORT/$DB?sslmode=disable"
export GOTOOLCHAIN TS_PYTHON TS_REQUIRE_PYTHON TEST_DATABASE_URL
echo "isolated PostgreSQL $NAME ($ID) on 127.0.0.1:$PORT, image $IMAGE" >&2
if [ -n "${TEST_RUN:-}" ]; then
  set -- -run "$TEST_RUN" "$@"
fi
go test -race -count=1 -p 1 -v "$@"
