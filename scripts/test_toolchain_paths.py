"""Regression tests for path exchange with scripts/toolchain.sh: paths pass by
plain assignment only, so quotes or shell syntax in a checkout path stay text.

Uses a fake checkout inside a temporary folder whose name holds an apostrophe and
a harmless marker, $(printf INJ%sCTED E >&2). Only a shell that re-parses the
path would print the assembled word INJECTED; the path text itself never holds
that word, so finding it proves re-parsing. Fake uv and python scripts stand in for the toolchain:
nothing is installed or downloaded. Also checks that no script uses eval.
Run from the repo root:
    python3 -B scripts/test_toolchain_paths.py
"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
MARKER = "INJECTED"
MARKER_CODE = "$(printf INJ%sCTED E >&2)"
FAKE_UV = """#!/bin/sh
# Fake uv: "python find" prints the fake interpreter in this checkout's .tools.
case "$1 $2" in
  "python find") printf '%s\\n' "$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)/python/bin/python3.14" ;;
  *) exit 3 ;;
esac
"""


def fake_checkout(parent, version="3.14.8", with_uv=True):
    root = parent / ("check it's %s out" % MARKER_CODE) / "repo"
    (root / "scripts").mkdir(parents=True)
    for name in ["toolchain.sh", "toolchain-paths.sh"]:
        shutil.copy2(ROOT / "scripts" / name, root / "scripts" / name)
    (root / "research").mkdir()
    (root / "research" / ".python-version").write_text("3.14.8\n")
    python = root / ".tools" / "python" / "bin" / "python3.14"
    python.parent.mkdir(parents=True)
    python.write_text("#!/bin/sh\necho %s\n" % version)
    python.chmod(0o700)
    if with_uv:
        uv = root / ".tools" / "uv-venv" / "bin" / "uv"
        uv.parent.mkdir(parents=True)
        uv.write_text(FAKE_UV)
        uv.chmod(0o700)
    return root, python


def sh(script, *args):
    return subprocess.run(["sh", "-c", script, "sh", *map(str, args)], capture_output=True, text=True, timeout=30)


class ToolchainPathTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.tmp = Path(os.path.realpath(temp.name))

    def assertNoInjection(self, result):
        self.assertNotIn(MARKER, result.stdout + result.stderr)

    def test_paths_with_quotes_and_shell_syntax_stay_text(self):
        root, python = fake_checkout(self.tmp)
        for mode, want in [("python", python), ("uv", root / ".tools" / "uv-venv" / "bin" / "uv")]:
            with self.subTest(mode=mode):
                result = subprocess.run([str(root / "scripts" / "toolchain.sh"), mode], capture_output=True, text=True, timeout=30)
                self.assertEqual((result.returncode, result.stdout), (0, "%s\n" % want), result.stderr)
                self.assertNoInjection(result)

    def test_launcher_assignment_keeps_the_exact_path(self):
        root, python = fake_checkout(self.tmp)
        result = sh('set -eu; TS_PYTHON=$("$1/scripts/toolchain.sh" python); export TS_PYTHON; printf "%s" "$TS_PYTHON"', root)
        self.assertEqual((result.returncode, result.stdout), (0, str(python)), result.stderr)
        self.assertNoInjection(result)

    def test_sourced_paths_stay_text(self):
        root, _ = fake_checkout(self.tmp)
        result = sh('ROOT=$1; . "$ROOT/scripts/toolchain-paths.sh"; printf "%s\\n%s\\n%s\\n" "$UV" "$UV_CACHE_DIR" "$UV_PROJECT_ENVIRONMENT"', root)
        tools = root / ".tools"
        self.assertEqual(result.stdout.splitlines(), [str(tools / "uv-venv" / "bin" / "uv"), str(tools / "uv-cache"), str(tools / "research-venv")])
        self.assertNoInjection(result)

    def test_missing_or_wrong_toolchain_stops_the_launcher(self):
        for kwargs, message in [({"with_uv": False}, "toolchain missing"), ({"version": "3.9.6"}, "want 3.14.8")]:
            with self.subTest(**kwargs):
                parent = self.tmp / ("case-%s" % len(list(self.tmp.iterdir())))
                parent.mkdir()
                root, _ = fake_checkout(parent, **kwargs)
                result = sh('set -eu; TS_PYTHON=$("$1/scripts/toolchain.sh" python); echo REACHED', root)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn("REACHED", result.stdout)
                self.assertIn(message, result.stderr)
                self.assertNoInjection(result)


class NoEvalTests(unittest.TestCase):
    EVAL = re.compile(r"(^|[\s;&|(`])eval(\s|$)")

    def test_no_script_uses_eval(self):
        for path in sorted((ROOT / "scripts").glob("*.sh")) + [ROOT / "Makefile"]:
            code = [line for line in path.read_text().splitlines() if not line.lstrip().startswith("#")]
            with self.subTest(script=path.name):
                self.assertEqual([line for line in code if self.EVAL.search(line)], [])

    def test_pattern_catches_eval(self):
        self.assertTrue(self.EVAL.search('eval "$X"'))
        self.assertTrue(self.EVAL.search('a; eval "$(cmd)"'))
        self.assertFalse(self.EVAL.search("# re-evaluate later; evaluation"))


if __name__ == "__main__":
    unittest.main()
