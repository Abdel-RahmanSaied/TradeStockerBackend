#!/bin/sh
# Optional synthetic demo setup. No real providers, funds or orders.
set -eu
cd "$(dirname "$0")/.."
set -a
. ./.env.local
set +a
go run ./cmd/market-ingestion
python3 -B - <<'PY'
import json,os,urllib.request,urllib.error
for currency,amount in [('USD',1000000),('EGP',10000000)]:
    request=urllib.request.Request('http://127.0.0.1:8080/api/v1/paper/accounts',method='POST',data=json.dumps({'currency':currency,'amount_minor':amount}).encode(),headers={'Authorization':'Bearer '+os.environ['TS_LOCAL_DEV_TOKEN'],'Content-Type':'application/json','Idempotency-Key':'demo-account-v1-'+currency})
    try:
        with urllib.request.urlopen(request) as response: json.load(response)
    except urllib.error.HTTPError as error:
        result=json.load(error)
        if error.code!=409 or result.get('error',{}).get('code')!='ACCOUNT_EXISTS': raise
print('Paper demo accounts ready. All funds and observations are synthetic. Live lanes remain disabled.')
PY
