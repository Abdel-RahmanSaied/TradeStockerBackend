#!/bin/sh
# Documented real SEC fetch through the real API, against a disposable database.
# Makes two requests to data.sec.gov with the approved TS_SEC_USER_AGENT. The
# normal paper database and ports 8080/5173 are not used. Usage:
#   scripts/research-smoke.sh [SYMBOL] [10-digit CIK]
set -eu
cd "$(dirname "$0")/.."
TS_REPO_ROOT=$(pwd)
set -a
. ./.env.local
set +a
[ -n "${TS_SEC_USER_AGENT:-}" ] || { printf 'Set the approved TS_SEC_USER_AGENT in .env.local first.\n' >&2; exit 1; }
. ./scripts/disposable-api.sh
disposable_api_preflight 18080
# A per-run token: only this run's API accepts it; .env.local is not changed.
disposable_api_fresh_token
TS_SMOKE_SYMBOL="${1:-AAPL}"
TS_SMOKE_CIK="${2:-0000320193}"
export TS_SMOKE_SYMBOL TS_SMOKE_CIK
TS_TEST_DB="tradestocker_test_$(python3 -c 'import secrets;print(secrets.token_hex(6))')"
TS_SERVER_PID=''
cleanup() {
  cd "$TS_REPO_ROOT"
  if [ -n "$TS_SERVER_PID" ]; then kill "$TS_SERVER_PID" 2>/dev/null || true; wait "$TS_SERVER_PID" 2>/dev/null || true; fi
  docker compose --env-file .env.local exec -T postgres dropdb -U tradestocker --force "$TS_TEST_DB" >/dev/null
}
docker compose --env-file .env.local exec -T postgres createdb -U tradestocker "$TS_TEST_DB"
trap cleanup EXIT INT TERM
mkdir -p bin .local
go build -o bin/platform-core ./cmd/platform-core
DATABASE_URL="postgres://tradestocker:${TS_DB_PASSWORD}@127.0.0.1:55432/${TS_TEST_DB}?sslmode=disable" TS_BIND=127.0.0.1:18080 ./bin/platform-core >.local/research-smoke-server.log 2>&1 &
TS_SERVER_PID=$!
disposable_api_wait 18080 "$TS_SERVER_PID" || { printf 'See .local/research-smoke-server.log\n' >&2; exit 1; }
python3 -B - <<'PY'
import hashlib, json, os, time, urllib.error, urllib.request

base = 'http://127.0.0.1:18080/api/v1'
auth = {'Authorization': 'Bearer ' + os.environ['TS_LOCAL_DEV_TOKEN']}

def call(path, body=None, key=None, raw=False):
    headers = dict(auth)
    if body is not None:
        headers.update({'Content-Type': 'application/json', 'Idempotency-Key': key})
    req = urllib.request.Request(base + path, data=None if body is None else json.dumps(body).encode(), headers=headers, method='POST' if body is not None else 'GET')
    with urllib.request.urlopen(req, timeout=30) as r:
        data = r.read()
    return data if raw else json.loads(data)

status = call('/research/status')
if not status['configured']:
    raise SystemExit('API reports the SEC contact as not configured')
job = call('/research/jobs', {'symbol': os.environ['TS_SMOKE_SYMBOL'], 'cik': os.environ['TS_SMOKE_CIK']}, key='smoke-' + str(int(time.time())))
deadline = time.time() + 180
while True:
    detail = call('/research/jobs/' + job['id'])
    if detail['job']['status'] in ('SUCCEEDED', 'FAILED') or time.time() > deadline:
        break
    time.sleep(1)
j = detail['job']
verified = []
for s in detail['sources']:
    data = call('/research/jobs/%s/sources/%s?sha256=%s' % (j['id'], s['kind'], s['sha256']), raw=True)
    ok = hashlib.sha256(data).hexdigest() == s['sha256'] and len(data) == s['size_bytes']
    verified.append({'kind': s['kind'], 'url': s['url'], 'sha256': s['sha256'], 'size_bytes': s['size_bytes'], 'received_at': s['received_at'], 'download_matches': ok})
summary = {'job': {k: j.get(k) for k in ('id', 'symbol', 'cik', 'status', 'attempts', 'error_code', 'created_at', 'updated_at')},
           'events': [e['action'] for e in detail['events']], 'sources': verified, 'parser_version': status['parser_version']}
r = j.get('result')
if r:
    def fact(p, m):
        return next((f['value'] for f in p['facts'] if f['metric'] == m), None)
    summary['report'] = {
        'issuer': r['issuer'], 'status': r['status'], 'outcome': r['outcome'], 'eligible_for_entry': r['eligible_for_entry'],
        'eligible_for_backtest': r['eligible_for_backtest'], 'as_of': r['as_of'], 'warnings': r['warnings'], 'excluded_facts': r['excluded_facts'],
        'periods': [{'fiscal_period_end': p['fiscal_period_end'], 'start': p['start'], 'accession': p['filing']['accession'], 'form': p['filing']['form'],
                     'filed': p['filing']['filed'], 'accepted_at': p['filing']['accepted_at'], 'revenue_usd': fact(p, 'revenue'),
                     'net_income_usd': fact(p, 'net_income'), 'assets_usd': fact(p, 'assets'),
                     'checks': {c['name']: c['status'] for c in p['checks']}, 'missing': p['missing_metrics'],
                     'withheld': len(p['withheld_metrics']), 'later_differences': len(p['later_filing_differences'])} for p in r['periods']]}
print(json.dumps(summary, indent=1))
if j['status'] != 'SUCCEEDED' or not all(s['download_matches'] for s in verified):
    raise SystemExit('Smoke run did not succeed with verified sources')
PY
