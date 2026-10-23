"""Regression tests for scripts/dev_stack_api.py, using fake servers this test
owns on free loopback ports and a synthetic token. No real API, credential or
outside request is involved. Run from the repo root:
    python3 -B scripts/test_dev_stack_api.py
"""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import threading
import unittest

# Normal runs test the reviewed client next to this file; the Makefile clears
# DEV_STACK_API_CLIENT, which only a deliberate red control sets.
CLIENT = Path(os.environ.get("DEV_STACK_API_CLIENT", Path(__file__).with_name("dev_stack_api.py")))
print("testing client: %s" % CLIENT, file=sys.stderr)
TOKEN = "t" * 64
FIXTURE = Path(__file__).resolve().parents[1] / "fixtures" / "synthetic" / "paper-accounts.json"


def serve(handler):
    server = ThreadingHTTPServer(("127.0.0.1", 0), handler)
    threading.Thread(target=server.serve_forever, daemon=True).start()
    return server


class FakeAPI(BaseHTTPRequestHandler):
    """Answers like platform-core; records every request it receives."""
    accounts = []
    seen = []
    redirect = None  # when set: the URL of the owned hostile server to redirect to

    def log_message(self, *args):
        pass

    def reply(self, body, status=200):
        data = json.dumps(body).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(data)))
        self.end_headers()
        self.wfile.write(data)

    def do_GET(self):
        FakeAPI.seen.append(("GET", self.path, self.headers.get("Authorization"), None))
        if FakeAPI.redirect:
            self.send_response(302)
            self.send_header("Location", FakeAPI.redirect)
            self.end_headers()
            return
        bodies = {
            "/api/v1/health": {"status": "ok", "mode": "PAPER", "live_advisory_enabled": False},
            "/api/v1/dashboard": {"accounts": FakeAPI.accounts, "quotes": [{"mode": "SYNTHETIC"}], "recommendations": []},
            "/api/v1/research/status": {"configured": False},
        }
        self.reply(bodies.get(self.path, {}), 200 if self.path in bodies else 404)

    def do_POST(self):
        body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
        FakeAPI.seen.append(("POST", self.path, self.headers.get("Authorization"), self.headers.get("Idempotency-Key")))
        FakeAPI.accounts.append({"currency": body["currency"], "cash_minor": body["amount_minor"]})
        self.reply({"ok": True})


class HostileProxy(BaseHTTPRequestHandler):
    """Stands in for an inherited proxy; it must never receive a request."""
    seen = []

    def log_message(self, *args):
        pass

    def do_GET(self):
        HostileProxy.seen.append(self.path)
        self.send_response(502)
        self.end_headers()

    do_POST = do_CONNECT = do_GET


class DevStackAPIClientTests(unittest.TestCase):
    def setUp(self):
        FakeAPI.accounts, FakeAPI.seen, FakeAPI.redirect, HostileProxy.seen = [], [], None, []
        self.api = serve(FakeAPI)
        self.proxy = serve(HostileProxy)
        self.addCleanup(self.api.shutdown)
        self.addCleanup(self.proxy.shutdown)
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.token_file = Path(temp.name) / "api_token"
        self.token_file.write_text(TOKEN)
        self.api_url = "http://127.0.0.1:%d" % self.api.server_address[1]
        proxy = "http://127.0.0.1:%d" % self.proxy.server_address[1]
        # Every proxy variable urllib reads, and no bypass list.
        self.env = {k: v for k, v in os.environ.items() if k.lower() not in ("no_proxy",)}
        self.env.update({name: proxy for name in ["http_proxy", "HTTP_PROXY", "all_proxy", "ALL_PROXY"]})

    def run_client(self, *args, api=None, token_file=None):
        return subprocess.run([sys.executable, "-I", str(CLIENT), *args, "--api", api or self.api_url, "--token-file", str(token_file or self.token_file)],
                              capture_output=True, text=True, env=self.env, timeout=30)

    def test_inherited_proxy_receives_nothing(self):
        out = self.run_client("summary")
        self.assertEqual(out.returncode, 0, out.stderr)
        self.assertEqual(HostileProxy.seen, [])
        self.assertEqual([path for _, path, _, _ in FakeAPI.seen], ["/api/v1/health", "/api/v1/dashboard", "/api/v1/research/status"])
        self.assertTrue(all(auth == "Bearer " + TOKEN for _, _, auth, _ in FakeAPI.seen))
        summary = json.loads(out.stdout)
        self.assertEqual((summary["mode"], summary["live_advisory_enabled"], summary["research_configured"]), ("PAPER", False, False))
        self.assertNotIn(TOKEN, out.stdout + out.stderr)

    def test_seed_creates_only_missing_accounts_with_fixed_keys(self):
        FakeAPI.accounts = [{"currency": "USD", "cash_minor": 1}]
        first = self.run_client("seed-accounts", "--fixture", str(FIXTURE))
        self.assertEqual(first.returncode, 0, first.stderr)
        posts = [(path, key) for method, path, _, key in FakeAPI.seen if method == "POST"]
        self.assertEqual(posts, [("/api/v1/paper/accounts", "synthetic-fixture-account-v1-EGP")])
        second = self.run_client("seed-accounts", "--fixture", str(FIXTURE))
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(len([s for s in FakeAPI.seen if s[0] == "POST"]), 1, "a second seed must change nothing")
        self.assertEqual(HostileProxy.seen, [])

    def test_redirect_to_an_owned_trap_is_not_followed(self):
        FakeAPI.redirect = "http://127.0.0.1:%d/trap" % self.proxy.server_address[1]
        out = self.run_client("summary")
        self.assertNotEqual(out.returncode, 0)
        self.assertEqual(len(FakeAPI.seen), 1)
        self.assertEqual(HostileProxy.seen, [], "the redirect target received a request")

    def test_non_loopback_targets_are_refused_before_any_request(self):
        # The token file does not exist: if the address check were removed, the
        # client would fail reading it, still before any network use, and the
        # expected message below would be missing.
        absent = self.token_file.with_name("absent-token")
        for api in ["http://localhost:18280", "https://127.0.0.1:18280", "http://10.0.0.1:18280", "http://127.0.0.1:18280/x"]:
            with self.subTest(api=api):
                refused = self.run_client("summary", api=api, token_file=absent)
                self.assertNotEqual(refused.returncode, 0)
                self.assertIn("only http://127.0.0.1", refused.stderr)
        self.assertEqual((FakeAPI.seen, HostileProxy.seen), ([], []))


if __name__ == "__main__":
    unittest.main()
