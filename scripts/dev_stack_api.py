"""Small client for the development stack's API, used by scripts/dev-stack.sh.

Standard library only. Accepts only an http://127.0.0.1:<port> API, ignores any
proxy settings in the environment, follows no redirects, reads the token from a
file and never prints it.
    python3 -I scripts/dev_stack_api.py summary       --api URL --token-file FILE
    python3 -I scripts/dev_stack_api.py has-quotes    --api URL --token-file FILE   (exit 0 yes, 3 no)
    python3 -I scripts/dev_stack_api.py seed-accounts --api URL --token-file FILE --fixture FILE
seed-accounts creates only the fixture accounts whose currency has no paper
account yet, each with its fixed idempotency key, so a second run changes nothing.
"""
import argparse
import json
import re
import sys
import urllib.error
import urllib.request

LOOPBACK = re.compile(r"^http://127\.0\.0\.1:[1-9][0-9]{1,4}$")


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise urllib.error.URLError("redirects are refused")


# An empty ProxyHandler: an inherited http_proxy must never receive the token.
OPENER = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect)


def call(api, token, method, path, body=None, key=None):
    headers = {"Authorization": "Bearer " + token}
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers["Content-Type"] = "application/json"
        headers["Idempotency-Key"] = key
    request = urllib.request.Request(api + path, data=data, method=method, headers=headers)
    with OPENER.open(request, timeout=10) as response:
        return json.load(response)


def summary(api, token):
    health = call(api, token, "GET", "/api/v1/health")
    dashboard = call(api, token, "GET", "/api/v1/dashboard")
    research = call(api, token, "GET", "/api/v1/research/status")
    return {
        "status": health.get("status"),
        "mode": health.get("mode"),
        "live_advisory_enabled": health.get("live_advisory_enabled"),
        "research_configured": research.get("configured"),
        "accounts": sorted((a["currency"], a["cash_minor"]) for a in dashboard.get("accounts") or []),
        "quotes": len(dashboard.get("quotes") or []),
        "quote_modes": sorted({q.get("mode") for q in dashboard.get("quotes") or []}),
        "recommendations": len(dashboard.get("recommendations") or []),
        # A read after this instant writes the expiry (paper read path), so a
        # persistence check must finish before it.
        "next_expiry": min((r["expires_at"] for r in dashboard.get("recommendations") or []
                            if r.get("status") in ("PROPOSED", "ACKNOWLEDGED")), default=None),
    }


def seed_accounts(api, token, fixture):
    with open(fixture) as f:
        accounts = json.load(f)
    if "SYNTHETIC" not in accounts.get("notice", ""):
        raise SystemExit("fixture is not labelled SYNTHETIC; refusing")
    existing = {a["currency"] for a in call(api, token, "GET", "/api/v1/dashboard").get("accounts") or []}
    done = []
    for account in accounts["accounts"]:
        if account["currency"] in existing:
            done.append({"currency": account["currency"], "result": "skipped: an account already exists"})
            continue
        call(api, token, "POST", "/api/v1/paper/accounts", {"currency": account["currency"], "amount_minor": account["amount_minor"]}, account["idempotency_key"])
        done.append({"currency": account["currency"], "result": "created"})
    return done


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    parser.add_argument("command", choices=["summary", "has-quotes", "seed-accounts"])
    parser.add_argument("--api", required=True)
    parser.add_argument("--token-file", required=True)
    parser.add_argument("--fixture")
    args = parser.parse_args(argv)
    if not LOOPBACK.match(args.api):
        raise SystemExit("only http://127.0.0.1:<port> is accepted")
    with open(args.token_file) as f:
        token = f.read().strip()
    if len(token) < 24:
        raise SystemExit("token file holds no usable token")
    if args.command == "summary":
        print(json.dumps(summary(args.api, token), sort_keys=True))
    elif args.command == "has-quotes":
        return 0 if call(args.api, token, "GET", "/api/v1/dashboard").get("quotes") else 3
    else:
        if not args.fixture:
            raise SystemExit("--fixture is required for seed-accounts")
        print(json.dumps(seed_accounts(args.api, token, args.fixture), sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
