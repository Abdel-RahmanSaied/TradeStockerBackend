#!/bin/sh
# The pinned research toolchain, inside .tools/ only (nothing global):
# uv from tools/uv-requirements.txt (hash-checked) and the exact CPython in
# research/.python-version, plus the research package environment. It only
# creates or updates files under .tools/; it never deletes anything.
#
#   scripts/toolchain.sh          install or verify; messages on stderr
#   scripts/toolchain.sh python   print the interpreter path (no install)
#   scripts/toolchain.sh uv       print the uv path (no install)
#
# Callers take a path with a plain assignment, never eval:
#   TS_PYTHON=$(./scripts/toolchain.sh python)
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
. "$ROOT/scripts/toolchain-paths.sh"
WANT=$(cat "$ROOT/research/.python-version")

interpreter() {
  [ -x "$UV" ] || { echo "toolchain missing: run scripts/toolchain.sh" >&2; exit 1; }
  py=$("$UV" python find --no-project "$WANT" 2>/dev/null) || { echo "CPython $WANT missing: run scripts/toolchain.sh" >&2; exit 1; }
  got=$("$py" -I -c 'import sys; print("%d.%d.%d" % sys.version_info[:3])')
  [ "$got" = "$WANT" ] || { echo "found CPython $got, want $WANT" >&2; exit 1; }
  printf '%s\n' "$py"
}

case "${1:-install}" in
  python) interpreter; exit 0 ;;
  uv) [ -x "$UV" ] || { echo "toolchain missing: run scripts/toolchain.sh" >&2; exit 1; }; printf '%s\n' "$UV"; exit 0 ;;
  install) ;;
  *) echo "usage: scripts/toolchain.sh [install|python|uv]" >&2; exit 2 ;;
esac

if [ ! -x "$UV" ]; then
  python3 -m venv "$TOOLS/uv-venv"
  "$TOOLS/uv-venv/bin/pip" install --quiet --disable-pip-version-check --no-cache-dir --require-hashes --only-binary :all: -r "$ROOT/tools/uv-requirements.txt"
fi
"$UV" python install "$WANT" --no-bin >&2
"$UV" sync --project "$ROOT/research" --locked --python "$WANT" >&2
echo "toolchain ready: $("$UV" --version), CPython $WANT at $(interpreter)" >&2
