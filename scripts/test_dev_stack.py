"""Regression tests for scripts/dev-stack.sh, run inside a fake repository.

The real dev-stack.sh, disposable-api.sh and compose.yaml are copied into a
temporary folder this test owns. docker and the API client are record-only
stubs on PATH: they write their arguments to a file and run nothing, so no
container, volume or network is touched, even by destroy-volumes. Run from the
repo root:
    python3 -B scripts/test_dev_stack.py
"""
import json
import os
from pathlib import Path
import re
import shutil
import socket
import stat
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[1]
DOCKER_STUB = """#!/bin/sh
exec python3 -I -c '
import json, os, sys
args = sys.argv[1:]
with open(os.environ["STUB_RECORD"], "a") as f:
    f.write(json.dumps({"tool": "docker", "args": args, "env": {k: os.environ.get(k) for k in
            ("TS_DEV_PROJECT", "TS_DEV_API_PORT", "TS_DEV_FRONTEND_PORT", "TS_DEV_SECRETS")}}) + "\\n")
if "ps" not in args:
    print("fake docker progress output")
if "test" in args:
    sys.exit(int(os.environ.get("STUB_TEST_EXIT", "1")))
' "$@"
"""
API_STUB = """import json, os, sys
with open(os.environ["STUB_RECORD"], "a") as f:
    f.write(json.dumps({"tool": "api", "args": sys.argv[1:]}) + "\\n")
if sys.argv[1] == "has-quotes":
    sys.exit(int(os.environ.get("STUB_HAS_QUOTES", "3")))
print("{}")
"""
NO_DOCKER = "no docker call may happen"


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


class DevStackTests(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.tmp = Path(os.path.realpath(temp.name))
        self.repo = self.tmp / "fake-repo"
        for rel in ["scripts/dev-stack.sh", "scripts/disposable-api.sh", "deploy/compose.yaml", "fixtures/synthetic/paper-accounts.json"]:
            (self.repo / rel).parent.mkdir(parents=True, exist_ok=True)
            shutil.copy2(ROOT / rel, self.repo / rel)
        (self.repo / "scripts" / "dev_stack_api.py").write_text(API_STUB)
        bin_dir = self.tmp / "stub-bin"
        bin_dir.mkdir()
        (bin_dir / "docker").write_text(DOCKER_STUB)
        (bin_dir / "docker").chmod(0o700)
        self.record = self.tmp / "record.jsonl"
        self.api_port = free_port()
        self.env = {"PATH": "%s:/usr/bin:/bin" % bin_dir, "HOME": str(self.tmp / "home"), "STUB_RECORD": str(self.record),
                    "TS_DEV_API_PORT": str(self.api_port), "TS_DEV_FRONTEND_PORT": str(free_port())}

    def stack(self, *args, **env):
        return subprocess.run([str(self.repo / "scripts" / "dev-stack.sh"), *args], capture_output=True, text=True,
                              env={**self.env, **env}, timeout=60)

    def calls(self, tool=None):
        if not self.record.exists():
            return []
        rows = [json.loads(line) for line in self.record.read_text().splitlines()]
        return [r for r in rows if tool is None or r["tool"] == tool]

    def compose_args(self, row):
        """The arguments after docker compose --project-name P --file F."""
        args = row["args"]
        self.assertEqual(args[:5], ["compose", "--project-name", row["env"]["TS_DEV_PROJECT"], "--file", str(self.repo / "deploy" / "compose.yaml")])
        return args[5:]

    def assertRefused(self, result, message):
        self.assertNotEqual(result.returncode, 0, result)
        self.assertIn(message, result.stderr)
        self.assertEqual(self.calls(), [], NO_DOCKER)
        self.assertFalse((self.repo / ".local").exists(), "a refused run must create nothing")

    def test_unsafe_project_names_are_refused(self):
        for name in ["tradestocker-local", "tradestocker-spike-s00t05", "TRADESTOCKER-DEV", "tradestocker-dev_x", "tradestocker-devx",
                     "tradestocker-dev-", "tradestocker-dev-x\nevil", "tradestocker-dev-$(printf INJ%sCTED E >&2)", "", "tradestocker-dev-" + "a" * 40]:
            with self.subTest(name=name):
                result = self.stack("status", TS_DEV_PROJECT=name)
                self.assertRefused(result, "TS_DEV_PROJECT" if "\n" not in name else "single line")
                self.assertNotIn("INJECTED", result.stdout + result.stderr)

    def test_unsafe_ports_are_refused(self):
        for port in ["8080", "5173", "55432", "18090", "15173", "80", "1000", "1023", "70000", "18280x", ""]:
            with self.subTest(port=port):
                self.assertRefused(self.stack("status", TS_DEV_API_PORT=port), "TS_DEV_API_PORT")
        self.assertRefused(self.stack("status", TS_DEV_FRONTEND_PORT=str(self.api_port)), "must differ")

    def test_config_file_values_are_text_not_code(self):
        (self.repo / "config").mkdir()
        (self.repo / "config" / "dev.env").write_text('TS_DEV_PROJECT=$(touch "%s")\n' % (self.tmp / "PWNED"))
        self.assertRefused(self.stack("status"), "TS_DEV_PROJECT")
        self.assertFalse((self.tmp / "PWNED").exists())

    def test_unknown_commands_and_extra_arguments_are_refused(self):
        for args in [["deploy"], ["down", "--volumes"], ["down", "-v"], ["stop", "now"], ["destroy-volumes", "all"], ["up", "--force"], ["up", "--no-seed", "x"]]:
            with self.subTest(args=args):
                self.assertRefused(self.stack(*args, TS_DEV_PROJECT="tradestocker-dev-review1", TS_DEV_CONFIRM="tradestocker-dev-review1"), "dev-stack: ")

    def test_down_and_stop_never_remove_volumes(self):
        for command in ["down", "stop"]:
            with self.subTest(command=command):
                result = self.stack(command)
                self.assertEqual(result.returncode, 0, result.stderr)
        rows = self.calls("docker")
        self.assertEqual([self.compose_args(r) for r in rows], [["down"], ["stop"]])
        self.assertEqual({r["env"]["TS_DEV_PROJECT"] for r in rows}, {"tradestocker-dev"})

    def test_destroy_volumes_needs_a_disposable_project_and_confirmation(self):
        self.assertRefused(self.stack("destroy-volumes", TS_DEV_CONFIRM="tradestocker-dev"), "never removed")
        self.assertRefused(self.stack("destroy-volumes", TS_DEV_PROJECT="tradestocker-dev-review1"), "TS_DEV_CONFIRM")
        self.assertRefused(self.stack("destroy-volumes", TS_DEV_PROJECT="tradestocker-dev-review1", TS_DEV_CONFIRM="tradestocker-dev-review2"), "TS_DEV_CONFIRM")
        result = self.stack("destroy-volumes", TS_DEV_PROJECT="tradestocker-dev-review1", TS_DEV_CONFIRM="tradestocker-dev-review1")
        self.assertEqual(result.returncode, 0, result.stderr)
        rows = self.calls("docker")
        self.assertEqual(len(rows), 1)
        self.assertEqual(rows[0]["env"]["TS_DEV_PROJECT"], "tradestocker-dev-review1")
        self.assertEqual(self.compose_args(rows[0]), ["down", "--volumes"])

    def test_first_up_creates_private_secrets_and_seeds_a_new_workspace(self):
        result = self.stack("up")
        self.assertEqual(result.returncode, 0, result.stderr)
        secrets = self.repo / ".local" / "dev-stack" / "tradestocker-dev"
        self.assertEqual(stat.S_IMODE(secrets.stat().st_mode), 0o700)
        files = sorted(p.name for p in secrets.iterdir())
        self.assertEqual(files, ["api_token", "database_url", "postgres_password"])
        for path in secrets.iterdir():
            self.assertEqual(stat.S_IMODE(path.stat().st_mode), 0o600)
            value = path.read_text()
            self.assertGreaterEqual(len(value), 48)
            self.assertNotIn(value, result.stdout + result.stderr)
        password = (secrets / "postgres_password").read_text()
        self.assertEqual((secrets / "database_url").read_text(), "postgres://tradestocker_dev:%s@dev-postgres:5432/tradestocker_dev?sslmode=disable" % password)
        docker = [self.compose_args(r) for r in self.calls("docker")]
        self.assertEqual(docker, [
            ["ps", "--status", "running", "--quiet", "platform-core"],
            ["build", "platform-core"],
            ["run", "--rm", "--no-deps", "dev-nats-volume"],
            ["up", "--detach", "--wait", "dev-postgres", "dev-nats", "platform-core"],
            ["run", "--rm", "market-ingestion"],
            ["run", "--rm", "--no-deps", "research-worker", "test", "-s", "/results/results.jsonl"],
            ["run", "--rm", "--no-deps", "research-worker"],
        ])
        self.assertEqual({r["env"]["TS_DEV_SECRETS"] for r in self.calls("docker")}, {str(secrets)})
        api = [r["args"][0] for r in self.calls("api")]
        self.assertEqual(api, ["has-quotes", "seed-accounts", "summary"])
        # stdout carries only the summary (the API stub prints "{}"); progress goes to stderr.
        self.assertEqual(result.stdout, "{}\n")
        self.assertIn("fake docker progress output", result.stderr)

    def test_restart_of_an_initialized_workspace_loads_nothing(self):
        self.assertEqual(self.stack("up").returncode, 0)
        secrets = self.repo / ".local" / "dev-stack" / "tradestocker-dev"
        before = {p.name: p.read_bytes() for p in secrets.iterdir()}
        self.record.write_text("")
        result = self.stack("up", STUB_HAS_QUOTES="0", STUB_TEST_EXIT="0")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual({p.name: p.read_bytes() for p in secrets.iterdir()}, before, "secrets must never be regenerated")
        docker = [self.compose_args(r) for r in self.calls("docker")]
        self.assertNotIn(["run", "--rm", "market-ingestion"], docker)
        self.assertNotIn(["run", "--rm", "--no-deps", "research-worker"], docker)

    def test_no_seed_loads_nothing(self):
        result = self.stack("up", "--no-seed")
        self.assertEqual(result.returncode, 0, result.stderr)
        docker = [self.compose_args(r) for r in self.calls("docker")]
        self.assertEqual(docker[-1], ["up", "--detach", "--wait", "dev-postgres", "dev-nats", "platform-core"])
        self.assertEqual([r["args"][0] for r in self.calls("api")], ["summary"])

    def test_damaged_secrets_are_refused(self):
        secrets = self.repo / ".local" / "dev-stack" / "tradestocker-dev-damaged"
        secrets.mkdir(parents=True, mode=0o700)
        (secrets / "api_token").write_text("x" * 64)
        (secrets / "api_token").chmod(0o600)
        result = self.stack("up", TS_DEV_PROJECT="tradestocker-dev-damaged")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("missing", result.stderr)
        self.assertEqual(sorted(p.name for p in secrets.iterdir()), ["api_token"])
        for name in ["postgres_password", "database_url"]:
            (secrets / name).write_text("y" * 64)
            (secrets / name).chmod(0o644)
        result = self.stack("up", TS_DEV_PROJECT="tradestocker-dev-damaged")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("mode 600", result.stderr)
        self.assertEqual(self.calls("docker"), [])

    def test_busy_api_port_is_refused_before_building(self):
        with socket.socket() as busy:
            busy.bind(("127.0.0.1", self.api_port))
            busy.listen()
            result = self.stack("up")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("busy", result.stderr)
        docker = [self.compose_args(r) for r in self.calls("docker")]
        self.assertEqual(docker, [["ps", "--status", "running", "--quiet", "platform-core"]])


class StaticSafetyTests(unittest.TestCase):
    def code(self, name):
        return [line for line in (ROOT / "scripts" / name).read_text().splitlines() if not line.lstrip().startswith("#")]

    def test_no_eval_and_no_file_deletion(self):
        for name in ["dev-stack.sh", "dev-stack-smoke.sh"]:
            with self.subTest(script=name):
                code = self.code(name)
                self.assertEqual([line for line in code if re.search(r"(^|[\s;&|(`])eval(\s|$)", line)], [])
                self.assertEqual([line for line in code if re.search(r"(^|[\s;&|(`])(rm|rmdir|unlink|shred)(\s|$)|-delete\b", line)], [])

    def test_volume_removal_exists_only_in_destroy_volumes(self):
        text = (ROOT / "scripts" / "dev-stack.sh").read_text()
        self.assertEqual(text.count("--volumes"), 1)
        block = text[text.index("  destroy-volumes)"):]
        self.assertIn("compose down --volumes", block[:block.index(";;")])
        self.assertNotRegex(text, r"down\s+-v\b")
        smoke = (ROOT / "scripts" / "dev-stack-smoke.sh").read_text()
        self.assertNotIn("--volumes", smoke)
        self.assertNotRegex(smoke, r"down\s+-v\b")


if __name__ == "__main__":
    unittest.main()
