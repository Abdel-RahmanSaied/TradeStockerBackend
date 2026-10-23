"""Query OSV (https://osv.dev) for every package in uv.lock. Prints JSON; exits 1 on any advisory."""
import json
import sys
import tomllib
import urllib.request

lock = tomllib.load(open("uv.lock", "rb"))
packages = [(p["name"], p["version"]) for p in lock["package"] if p.get("source", {}).get("registry")]
query = {"queries": [{"package": {"ecosystem": "PyPI", "name": n}, "version": v} for n, v in packages]}
request = urllib.request.Request("https://api.osv.dev/v1/querybatch", data=json.dumps(query).encode(), headers={"Content-Type": "application/json"})
with urllib.request.urlopen(request, timeout=60) as response:
    results = json.load(response)["results"]
advisories = {f"{n}=={v}": [x["id"] for x in r.get("vulns", [])] for (n, v), r in zip(packages, results) if r.get("vulns")}
print(json.dumps({"source": "https://api.osv.dev/v1/querybatch", "packages_checked": len(packages), "advisories": advisories}))
sys.exit(1 if advisories else 0)
