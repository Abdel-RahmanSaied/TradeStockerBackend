#!/bin/sh
# Negative controls for the CI baseline (S01-T03 validation). It clones the
# repository at COMMIT into a new temporary folder, checks that the CI targets
# pass there, then makes one deliberate break per branch of that clone and
# requires the matching make target to fail with that break's own diagnosis.
#
# The real repository is only read (rev-parse and git clone). Every edit,
# commit and branch is in the clone, which has no remote. Git there runs with
# private process settings only: no system, global or user configuration, an
# empty template (no hooks), no signing, no credential helper, no prompts. No
# real configuration is changed. Nothing is deleted: the clone, its logs and
# results.tsv stay in the printed folder for inspection.
#   scripts/ci_negative_controls.sh [COMMIT]     (default HEAD)
set -eu
unset GIT_DIR GIT_WORK_TREE GIT_INDEX_FILE GIT_OBJECT_DIRECTORY GIT_ALTERNATE_OBJECT_DIRECTORIES \
  GIT_NAMESPACE GIT_COMMON_DIR GIT_CONFIG GIT_CONFIG_PARAMETERS GIT_CONFIG_COUNT
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd -P)
COMMIT=$(git -C "$ROOT" rev-parse --verify "${1:-HEAD}^{commit}")
WORK=$(mktemp -d "${TMPDIR:-/tmp}/tradestocker-ci-negative.XXXXXX")
CLONE="$WORK/clone"
ISO="$WORK/isolation"
mkdir -p "$ISO/home" "$ISO/xdg" "$ISO/template" "$ISO/hooks"
# Go keeps using the existing, content-addressed module and build caches.
GOMODCACHE=$(go env GOMODCACHE)
GOCACHE=$(go env GOCACHE)
GOPATH=$(go env GOPATH)
export GOMODCACHE GOCACHE GOPATH
echo "ci-negative: commit $COMMIT, working folder $WORK" >&2

# iso CMD...: CMD with the private Git environment.
iso() {
  env HOME="$ISO/home" XDG_CONFIG_HOME="$ISO/xdg" GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
    GIT_TEMPLATE_DIR="$ISO/template" GIT_TERMINAL_PROMPT=0 GIT_ASKPASS=/bin/false SSH_ASKPASS=/bin/false "$@"
}
g() {
  iso git -c core.hooksPath="$ISO/hooks" -c commit.gpgsign=false -c tag.gpgsign=false -c credential.helper= "$@"
}

g clone --quiet --no-hardlinks --no-checkout --template="$ISO/template" "$ROOT" "$CLONE"
g -C "$CLONE" remote remove origin
for setting in "core.hooksPath $ISO/hooks" "commit.gpgsign false" "tag.gpgsign false" "credential.helper " \
               "user.name ci-negative-control" "user.email ci-negative-control@invalid"; do
  g -C "$CLONE" config ${setting%% *} "${setting#* }"
done
[ -z "$(g -C "$CLONE" remote)" ] || { echo "ci-negative: the clone still has a remote; stopping" >&2; exit 1; }
g -C "$CLONE" checkout --quiet -b neg-base "$COMMIT"
(cd "$CLONE" && iso ./scripts/toolchain.sh) >"$WORK/toolchain.log" 2>&1

# run NAME TARGET: the make target in the clone; prints its exit code.
run() {
  set +e
  (cd "$CLONE" && iso env -u TEST_DATABASE_URL make "$2" CI_OUT="$WORK/out-$1") >"$WORK/$1.log" 2>&1
  code=$?
  set -e
  echo "$code"
}

: > "$WORK/results.tsv"
for target in ci-go ci-python ci-docs ci-security; do
  code=$(run "baseline-$target" "$target")
  printf 'baseline\t%s\t%s\t%s\n' "$target" "$code" "$([ "$code" = 0 ] && echo PASS || echo FAIL)" >> "$WORK/results.tsv"
done

# control NAME TARGET EXPECTED-LOG-TEXT PYTHON-EDIT: on a new branch from
# neg-base, run the edit (cwd is the clone), commit it, run TARGET, and require
# a non-zero exit whose log contains EXPECTED-LOG-TEXT (a fixed string).
control() {
  g -C "$CLONE" switch --quiet -c "neg-$1" neg-base
  (cd "$CLONE" && python3 -I -c "$4")
  g -C "$CLONE" commit --quiet -am "negative control: $1"
  code=$(run "$1" "$2")
  if [ "$code" != 0 ] && grep -F -q -- "$3" "$WORK/$1.log"; then verdict=FAILED-AS-REQUIRED; else verdict=NOT-CAUGHT; fi
  printf '%s\t%s\t%s\t%s\n' "$1" "$2" "$code" "$verdict" >> "$WORK/results.tsv"
  g -C "$CLONE" switch --quiet neg-base
}

control go-test ci-go "deliberate negative control" "
from pathlib import Path
Path('internal/paper/service_test.go').open('a').write('\nfunc TestNegativeControl(t *testing.T) { t.Fatal(\"deliberate negative control\") }\n')"
control gofmt ci-go "gofmt needed: internal/paper/model.go" "
from pathlib import Path
Path('internal/paper/model.go').open('a').write('\nvar   negativeControl=1\n')"
control python-test ci-python "deliberate negative control" "
from pathlib import Path
Path('research/tests/test_worker.py').open('a').write('\n\nclass NegativeControl(unittest.TestCase):\n    def test_fails(self):\n        self.fail(\"deliberate negative control\")\n')"
control lint ci-python "F401" "
from pathlib import Path
p = Path('scripts/dev_stack_api.py'); p.write_text(p.read_text().replace('import argparse\n', 'import argparse\nimport os\n', 1))"
control types ci-python "Incompatible types in assignment" "
from pathlib import Path
Path('fixtures/synthetic/label.py').open('a').write('\nnegative_control: int = \"not a number\"\n')"
control fixture-drift ci-python "test_manifest_matches_every_fixture" "
from pathlib import Path
p = Path('fixtures/synthetic/paper-accounts.json'); p.write_text(p.read_text().replace('1000000', '1000001', 1))"
control state-schema ci-docs "S01-T03: invalid task status MAYBE" "
import json
from pathlib import Path
p = Path('docs/state/project_state.json'); s = json.loads(p.read_text()); s['tasks']['S01-T03']['status'] = 'MAYBE'; p.write_text(json.dumps(s, indent=2) + '\n')"
control generated-docs ci-docs "Generated docs drift: sprints/S00.md" "
from pathlib import Path
p = Path('docs/sprints/S00.md'); p.write_text(p.read_text() + '\nnegative control drift\n')"
control secret ci-security "FINDING: github-pat" "
import secrets, string
from pathlib import Path
token = 'ghp_' + ''.join(secrets.choice(string.ascii_letters + string.digits) for _ in range(36))
Path('docs/evidence/README.md').open('a').write('\nnegative control (invented, not a real token): ' + token + '\n')"
control expired-waiver ci-security "INVALID WAIVER: W-001: expired" "
import json
from pathlib import Path
p = Path('security/scan-waivers.json'); w = json.loads(p.read_text()); w['waivers'][0]['expires'] = '2026-01-01'; p.write_text(json.dumps(w, indent=2) + '\n')"

column -t -s "$(printf '\t')" "$WORK/results.tsv" >&2
echo "ci-negative: logs and clone kept in $WORK" >&2
if grep -q -E "	FAIL$|NOT-CAUGHT" "$WORK/results.tsv"; then
  echo "ci-negative: FAILED (a baseline target failed or a break was not caught)" >&2
  exit 1
fi
echo "ci-negative: PASS (baseline green; every break failed its target with its own diagnosis)" >&2
