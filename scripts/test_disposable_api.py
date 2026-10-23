"""Regression tests for scripts/disposable-api.sh, using fake servers on free ports.

No real API, database or SEC request is involved. Run from the repo root:
    python3 -B scripts/test_disposable_api.py
"""
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import threading
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import unittest

LIBRARY = Path(__file__).with_name("disposable-api.sh")
TOKEN = "f" * 64
STATUS = {"configured": False, "parser_version": "sec-annual-v2", "live_advisory_enabled": False}


def free_port():
    with socket.socket() as s:
        s.bind(("127.0.0.1", 0))
        return s.getsockname()[1]


def shell(command, *args, token=TOKEN):
    env = {**os.environ, "TS_LOCAL_DEV_TOKEN": token}
    return subprocess.run(["sh", "-c", '. "$0" && ' + command, str(LIBRARY), *args], capture_output=True, text=True, env=env, timeout=60)


class FakeServer:
    """A server that answers every request the same way, ignoring the token."""

    def __init__(self, status, body):
        reply = json.dumps(body).encode() if not isinstance(body, bytes) else body

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                self.send_response(status)
                self.send_header("Content-Type", "application/json")
                self.end_headers()
                self.wfile.write(reply)

            def log_message(self, *args):
                pass

        self.port = free_port()
        self.server = ThreadingHTTPServer(("127.0.0.1", self.port), Handler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()


# A stand-in for the real API: it listens on the port and accepts only TOKEN.
OWN_SERVER = r'''
import json, sys
from http.server import BaseHTTPRequestHandler, HTTPServer
class H(BaseHTTPRequestHandler):
    def do_GET(self):
        ok = self.headers.get("Authorization") == "Bearer " + sys.argv[2]
        self.send_response(200 if ok else 401)
        self.end_headers()
        self.wfile.write(json.dumps(%s).encode() if ok else b"{}")
    def log_message(self, *a): pass
HTTPServer(("127.0.0.1", int(sys.argv[1])), H).serve_forever()
''' % repr(STATUS)


class Preflight(unittest.TestCase):
    def test_busy_port_is_refused_and_free_port_passes(self):
        busy = socket.socket()
        busy.bind(("127.0.0.1", 0))
        busy.listen()
        port = busy.getsockname()[1]
        try:
            result = shell('disposable_api_preflight "$1"', str(port))
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("is busy", result.stderr)
        finally:
            busy.close()
        self.assertEqual(shell('disposable_api_preflight "$1"', str(port)).returncode, 0)

    def test_every_listed_port_is_checked(self):
        server = FakeServer(200, STATUS)
        try:
            result = shell('disposable_api_preflight "$1" "$2"', str(free_port()), str(server.port))
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("Port %d is busy" % server.port, result.stderr)
        finally:
            server.close()


class TimeWait(unittest.TestCase):
    def test_time_wait_socket_is_not_mistaken_for_a_live_server(self):
        # The server closes first, which leaves its port in TIME_WAIT.
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen()
        port = listener.getsockname()[1]
        client = socket.create_connection(("127.0.0.1", port))
        accepted, _ = listener.accept()
        accepted.close()
        client.recv(1)
        client.close()
        listener.close()
        probe = socket.socket()
        try:
            probe.bind(("127.0.0.1", port))
            self.skipTest("this OS left no TIME_WAIT that blocks bind; nothing to compare")
        except OSError:
            pass  # A bind-based check would wrongly report this port as busy.
        finally:
            probe.close()
        result = shell('disposable_api_preflight "$1"', str(port))
        self.assertEqual(result.returncode, 0, result.stderr)


class DevLauncher(unittest.TestCase):
    def test_launcher_uses_the_shared_port_check_and_keeps_its_token(self):
        text = Path(__file__).with_name("dev.sh").read_text(encoding="utf-8")
        self.assertIn("disposable_api_preflight 8080 5173", text)
        self.assertNotIn("disposable_api_fresh_token", text)
        self.assertNotIn("s.bind(", text)
        self.assertIn("--strictPort", text)


class Readiness(unittest.TestCase):
    def wait(self, port, pid):
        return shell('disposable_api_wait "$1" "$2"', str(port), str(pid))

    def test_dead_child_with_old_server_answering_fails(self):
        # Reviewer reproduction: our API exits, an older server answers 200.
        server = FakeServer(200, STATUS)
        child = subprocess.Popen(["sh", "-c", "exit 1"])
        child.wait()
        try:
            result = self.wait(server.port, child.pid)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("exited during startup", result.stderr)
        finally:
            server.close()

    def test_zombie_child_counts_as_dead(self):
        server = FakeServer(200, STATUS)
        child = subprocess.Popen(["sh", "-c", "exit 1"])  # Not reaped: a zombie.
        try:
            result = self.wait(server.port, child.pid)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("exited during startup", result.stderr)
        finally:
            child.wait()
            server.close()

    @unittest.skipUnless(shutil.which("lsof"), "lsof not installed; the token check still applies")
    def test_live_child_that_does_not_own_the_port_fails(self):
        server = FakeServer(200, STATUS)  # Ignores the token, like a stray server.
        child = subprocess.Popen(["sleep", "30"])
        try:
            result = self.wait(server.port, child.pid)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("not the disposable API", result.stderr)
        finally:
            child.kill()
            child.wait()
            server.close()

    def test_server_rejecting_this_run_token_fails(self):
        server = FakeServer(401, {"error": {"code": "UNAUTHORIZED"}})
        child = subprocess.Popen(["sleep", "30"])
        try:
            result = self.wait(server.port, child.pid)
            self.assertNotEqual(result.returncode, 0)
            self.assertIn("rejected this run token", result.stderr)
        finally:
            child.kill()
            child.wait()
            server.close()

    def test_own_child_with_the_run_token_is_ready(self):
        port = free_port()
        child = subprocess.Popen([sys.executable, "-I", "-c", OWN_SERVER, str(port), TOKEN])
        try:
            result = self.wait(port, child.pid)
            self.assertEqual(result.returncode, 0, result.stderr)
            wrong = shell('disposable_api_wait "$1" "$2"', str(port), str(child.pid), token="0" * 64)
            self.assertNotEqual(wrong.returncode, 0)
            self.assertIn("rejected this run token", wrong.stderr)
        finally:
            child.kill()
            child.wait()


class FreshToken(unittest.TestCase):
    def test_each_run_gets_a_new_token_without_touching_files(self):
        before = {p: p.stat().st_mtime for p in Path.cwd().glob(".env*")}
        tokens = {shell('disposable_api_fresh_token && printf %s "$TS_LOCAL_DEV_TOKEN"').stdout for _ in range(3)}
        self.assertEqual(len(tokens), 3)
        self.assertTrue(all(len(t) == 64 and t != TOKEN for t in tokens))
        self.assertEqual(before, {p: p.stat().st_mtime for p in Path.cwd().glob(".env*")})


if __name__ == "__main__":
    unittest.main()
