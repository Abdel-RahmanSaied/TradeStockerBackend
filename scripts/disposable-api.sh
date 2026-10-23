# Shared helpers for scripts that start a disposable local API. Source this file.
#
# A disposable API gets a fresh random token for each run. It is ready only when
# its own process is alive, owns the listening socket and accepts that token, so
# an older server on the same port can never receive this run's requests. The
# normal TS_LOCAL_DEV_TOKEN in .env.local is never changed.

# disposable_api_fresh_token: export a new random token for this run only.
disposable_api_fresh_token() {
  TS_LOCAL_DEV_TOKEN=$(python3 -c 'import secrets; print(secrets.token_hex(32))')
  export TS_LOCAL_DEV_TOKEN
}

# disposable_api_preflight PORT...: fail if anything already answers on a port.
# Connecting (not binding) also catches wildcard listeners and ignores TIME_WAIT.
disposable_api_preflight() {
  python3 -B - "$@" <<'PY'
import socket, sys
for port in sys.argv[1:]:
    with socket.socket() as s:
        s.settimeout(0.5)
        if s.connect_ex(('127.0.0.1', int(port))) == 0:
            raise SystemExit('Port %s is busy. Stop the server using it, then retry.' % port)
PY
}

# disposable_api_wait PORT PID: wait until PID is alive, PID itself listens on
# PORT, and the server accepts this run's TS_LOCAL_DEV_TOKEN. Fails closed.
disposable_api_wait() {
  python3 -B - "$1" "$2" <<'PY'
import json, os, subprocess, sys, time, urllib.error, urllib.request
port, pid = int(sys.argv[1]), int(sys.argv[2])
token = os.environ['TS_LOCAL_DEV_TOKEN']

def alive():
    stat = subprocess.run(['ps', '-o', 'stat=', '-p', str(pid)], capture_output=True, text=True).stdout.strip()
    return bool(stat) and not stat.startswith('Z')

def listeners():
    try:
        out = subprocess.run(['lsof', '-nP', '-iTCP:%d' % port, '-sTCP:LISTEN', '-t'], capture_output=True, text=True, timeout=5).stdout
    except FileNotFoundError:
        return None  # Without lsof, the per-run token check below still applies.
    return {int(p) for p in out.split()}

for _ in range(150):
    if not alive():
        raise SystemExit('The disposable API (pid %d) exited during startup. Inspect its log.' % pid)
    request = urllib.request.Request('http://127.0.0.1:%d/api/v1/research/status' % port, headers={'Authorization': 'Bearer ' + token})
    try:
        with urllib.request.urlopen(request, timeout=1) as response:
            raw = response.read()
    except urllib.error.HTTPError as error:
        if error.code == 401:
            raise SystemExit('Another server on port %d rejected this run token.' % port)
        time.sleep(0.1)
        continue
    except OSError:
        time.sleep(0.1)
        continue
    owners = listeners()
    if owners is not None and owners != {pid}:
        raise SystemExit('Port %d is served by pid %s, not the disposable API (pid %d).' % (port, sorted(owners), pid))
    try:
        body = json.loads(raw)
    except ValueError:
        raise SystemExit('Port %d answered, but not as a TradeStocker API.' % port)
    if not isinstance(body, dict) or body.get('live_advisory_enabled') is not False or 'parser_version' not in body:
        raise SystemExit('Port %d answered, but not as a TradeStocker API.' % port)
    if not alive():
        raise SystemExit('The disposable API (pid %d) exited during startup. Inspect its log.' % pid)
    break
else:
    raise SystemExit('The disposable API did not become ready. Inspect its log.')
PY
}
