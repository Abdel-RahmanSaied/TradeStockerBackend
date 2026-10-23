#!/bin/sh
# Packages twice into two new folders, each with its own empty Go build cache,
# and requires byte-identical artifacts. Everything goes into one new temporary
# folder that is left in place; nothing is deleted.
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/tradestocker-reproducible.XXXXXX")
for run in a b; do
  GOCACHE="$WORK/gocache-$run" "$ROOT/scripts/package.sh" "$WORK/$run"
done
if cmp -s "$WORK/a/SHA256SUMS" "$WORK/b/SHA256SUMS"; then
  cat "$WORK/a/SHA256SUMS"
  echo "reproducible: both packages are byte-identical (kept in $WORK)" >&2
else
  diff "$WORK/a/SHA256SUMS" "$WORK/b/SHA256SUMS" >&2 || true
  echo "NOT reproducible (kept in $WORK)" >&2
  exit 1
fi
