"""Regression tests for the package output guard (scripts/package-out.sh) and
the refusal in scripts/package.sh.

Every target is inside a temporary folder this test creates: a fake repository,
a fake HOME and fake user folders. No real project, home or database path is
passed, and HOME is always the fake one. The end-to-end test runs a copy of
package.sh inside a fake repository, with record-only stubs for the toolchain,
go, uv and shasum, so even a broken guard reaches only test-owned paths. The
guard has no delete command; one test checks that the packaging scripts never
gain one. Run from the repo root:
    python3 -B scripts/test_package_out.py
PACKAGE_OUT_LIBRARY=<file> tests another guard file instead (only for a
record-only negative control kept outside the repository).
"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
LIBRARY = Path(os.environ.get("PACKAGE_OUT_LIBRARY", ROOT / "scripts" / "package-out.sh"))
PACKAGING_SCRIPTS = ["package-out.sh", "package.sh", "check-reproducible.sh"]


def snapshot(root):
    """Every path under root with its content (files) or link target (links)."""
    out = {}
    for path in sorted(Path(root).rglob("*")):
        rel = str(path.relative_to(root))
        if path.is_symlink():
            out[rel] = ("link", os.readlink(path))
        elif path.is_file():
            out[rel] = ("file", path.read_bytes())
        else:
            out[rel] = ("dir", None)
    return out


def write(path, text="sentinel"):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


class PackageOutputTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.tmp = Path(os.path.realpath(temp.name))
        self.home = self.tmp / "home"
        write(self.home / "notes.txt")
        self.repo = self.home / "work" / "repo"
        (self.repo / ".git").mkdir(parents=True)
        write(self.repo / "go.mod", "module tradestocker\n")
        write(self.repo / "cmd" / "main.go", "package main\n")

    def prepare(self, out, repo=None, cwd=None):
        env = {**os.environ, "HOME": str(self.home)}
        return subprocess.run(["sh", "-c", '. "$0" && prepare_out "$1" "$2"', str(LIBRARY), str(out), str(repo or self.repo)],
                              capture_output=True, text=True, env=env, cwd=cwd or self.tmp, timeout=30)

    def assertRefusedAndUntouched(self, out, **kwargs):
        before = snapshot(self.tmp)
        result = self.prepare(out, **kwargs)
        self.assertNotEqual(result.returncode, 0, result)
        self.assertIn("refusing", result.stderr)
        self.assertEqual(result.stdout, "")
        self.assertEqual(snapshot(self.tmp), before)
        return result

    def test_new_folder_is_created_and_printed(self):
        result = self.prepare(self.tmp / "out")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "%s\n" % (self.tmp / "out"))
        self.assertEqual(list((self.tmp / "out").iterdir()), [])

    def test_relative_path_is_resolved(self):
        result = self.prepare("relative-out")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(result.stdout, "%s\n" % (self.tmp / "relative-out"))

    def test_empty_folder_is_used_without_writing(self):
        (self.tmp / "empty").mkdir()
        result = self.prepare(self.tmp / "empty")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(list((self.tmp / "empty").iterdir()), [])

    def test_user_folder_with_content_is_refused_untouched(self):
        write(self.tmp / "user" / "report.pdf")
        write(self.tmp / "user" / "project" / "main.go", "package main")
        self.assertRefusedAndUntouched(self.tmp / "user")

    def test_repository_home_and_their_parents_are_refused(self):
        for target in [self.repo, self.repo.parent, self.home, self.tmp]:
            with self.subTest(target=target):
                result = self.assertRefusedAndUntouched(target)
                self.assertIn("one of its parent folders", result.stderr)

    def test_repository_is_refused_even_when_empty(self):
        empty_repo = self.tmp / "empty-repo"
        empty_repo.mkdir()
        result = self.assertRefusedAndUntouched(empty_repo, repo=empty_repo)
        self.assertIn("one of its parent folders", result.stderr)

    def test_inside_git_folder_is_refused(self):
        self.assertRefusedAndUntouched(self.repo / ".git" / "package")

    def test_links_files_and_missing_parents_are_refused(self):
        write(self.tmp / "user" / "keep.txt")
        (self.tmp / "link").symlink_to(self.tmp / "user")
        self.assertRefusedAndUntouched(self.tmp / "link")
        write(self.tmp / "plain-file")
        self.assertRefusedAndUntouched(self.tmp / "plain-file")
        self.assertRefusedAndUntouched(self.tmp / "no" / "such" / "out")

    def test_folder_holding_only_newline_named_entries_is_not_empty(self):
        for names in [["\n"], ["a\nb"], ["\n", ".\n"]]:
            with self.subTest(names=names):
                out = self.tmp / ("newline-entries-%d" % len(list(self.tmp.glob("newline-entries-*"))))
                out.mkdir()
                for name in names:
                    write(out / name)
                result = self.assertRefusedAndUntouched(out)
                self.assertIn("not empty", result.stderr)

    def test_newline_in_output_repository_or_home_is_refused(self):
        self.assertRefusedAndUntouched(self.tmp / "new\nline")
        self.assertRefusedAndUntouched(self.tmp / "trailing\n")
        odd_repo = self.tmp / "re\npo"
        (odd_repo / ".git").mkdir(parents=True)
        self.assertRefusedAndUntouched(self.tmp / "out", repo=odd_repo)
        odd_home = self.tmp / "ho\nme"
        odd_home.mkdir()
        before = snapshot(self.tmp)
        env = {**os.environ, "HOME": str(odd_home)}
        result = subprocess.run(["sh", "-c", '. "$0" && prepare_out "$1" "$2"', str(LIBRARY), str(self.tmp / "out"), str(self.repo)],
                                capture_output=True, text=True, env=env, cwd=self.tmp, timeout=30)
        self.assertNotEqual(result.returncode, 0, result)
        self.assertIn("newline", result.stderr)
        self.assertEqual(snapshot(self.tmp), before)

    @unittest.skipIf(os.geteuid() == 0, "root can write to any folder")
    def test_folder_that_is_not_writable_is_refused(self):
        readonly = self.tmp / "readonly"
        readonly.mkdir()
        readonly.chmod(0o500)
        self.addCleanup(readonly.chmod, 0o700)
        result = self.prepare(readonly)
        self.assertNotEqual(result.returncode, 0, result)
        self.assertIn("not writable", result.stderr)

    @unittest.skipIf(os.geteuid() == 0, "root can list any folder")
    def test_unlistable_folder_fails_closed(self):
        # Writable and searchable but not readable: only the listing can fail.
        locked = self.tmp / "locked"
        locked.mkdir()
        locked.chmod(0o300)
        self.addCleanup(locked.chmod, 0o700)
        result = self.prepare(locked)
        self.assertNotEqual(result.returncode, 0, result)
        self.assertIn("cannot be listed", result.stderr)

    def test_quotes_and_shell_syntax_in_paths_stay_text(self):
        # Run by a shell, the marker prints the assembled word INJECTED, which the
        # path text itself never contains.
        name = "it's $(printf INJ%sCTED E >&2) out"
        result = self.prepare(self.tmp / name)
        self.assertEqual((result.returncode, result.stdout), (0, "%s\n" % (self.tmp / name)), result.stderr)
        self.assertNotIn("INJECTED", result.stdout + result.stderr)
        self.assertTrue((self.tmp / name).is_dir())
        odd_repo = self.tmp / "repo ' $(printf INJ%sCTED E >&2)"
        (odd_repo / ".git").mkdir(parents=True)
        refused = self.assertRefusedAndUntouched(odd_repo, repo=odd_repo)
        self.assertNotIn("INJECTED", refused.stderr)

    def fake_package_repo(self):
        """A fake repository in the test folder: the real package.sh and toolchain
        paths plus the guard under test, and record-only stubs for every tool
        package.sh calls after the guard. A broken guard can reach only these."""
        repo = self.tmp / "fake-repo"
        (repo / ".git").mkdir(parents=True)
        write(repo / "go.mod", "module tradestocker\n\ngo 1.26.10\n")
        (repo / "scripts").mkdir()
        shutil.copy2(ROOT / "scripts" / "package.sh", repo / "scripts" / "package.sh")
        shutil.copy2(ROOT / "scripts" / "toolchain-paths.sh", repo / "scripts" / "toolchain-paths.sh")
        shutil.copy2(LIBRARY, repo / "scripts" / "package-out.sh")
        stub = '#!/bin/sh\nprintf "%s %s\\n" "$(basename "$0")" "$*" >> "$STUB_RECORD"\nexit 1\n'
        stubs = self.tmp / "stub-bin"
        for path in [repo / "scripts" / "toolchain.sh", stubs / "go", stubs / "uv", stubs / "shasum"]:
            write(path, stub)
            path.chmod(0o700)
        return repo, stubs

    def test_package_script_refuses_before_any_tool_runs(self):
        repo, stubs = self.fake_package_repo()
        write(self.tmp / "user" / "notes.txt")
        record = self.tmp / "stub-record.txt"
        before = snapshot(self.tmp)
        env = {**os.environ, "HOME": str(self.home), "TMPDIR": str(self.tmp), "STUB_RECORD": str(record), "PATH": "%s:/usr/bin:/bin" % stubs}
        for target in [self.tmp / "user", repo, self.home]:
            with self.subTest(target=target):
                result = subprocess.run([str(repo / "scripts" / "package.sh"), str(target)], capture_output=True, text=True, env=env, timeout=60)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("refusing", result.stderr)
                self.assertFalse(record.exists(), "a tool ran after the guard: %s" % (record.read_text() if record.exists() else ""))
                self.assertEqual(snapshot(self.tmp), before)


class NoDeleteCommandTests(unittest.TestCase):
    DELETE = re.compile(r"(^|[\s;&|(`])(rm|rmdir|unlink|shred)(\s|$)|-delete\b|rmtree")

    def test_packaging_scripts_contain_no_delete_command(self):
        for name in PACKAGING_SCRIPTS:
            code = [line for line in (ROOT / "scripts" / name).read_text().splitlines() if not line.lstrip().startswith("#")]
            with self.subTest(script=name):
                self.assertEqual([line for line in code if self.DELETE.search(line)], [])

    def test_pattern_catches_delete_commands(self):
        for line in ["rm -rf x", "  rm x", "a && rm -f y", "find . -delete", "(rmdir d)", "x; unlink f"]:
            with self.subTest(line=line):
                self.assertTrue(self.DELETE.search(line))
        self.assertFalse(self.DELETE.search('echo "format and term"'))


if __name__ == "__main__":
    unittest.main()
