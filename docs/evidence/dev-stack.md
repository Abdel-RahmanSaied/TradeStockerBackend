# Task evidence — S01-T02

Create local containers and deterministic fixtures.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S01-T02` / `claude-code-agent` (session `2026-10-23-development-8da61cee`) / **independent runtime review pending** |
| Base | Backend `fa954355042ba10bd9fdd406e02053aa527ebb8b` (state 61) |
| Code | The checkpoint commit that adds this record; no frontend change |
| Environment | macOS 27.0.1 arm64, Docker Desktop (Docker client and server 29.7.2, Compose 5.3.1), Go 1.26.10, CPython 3.14.8 |
| Logs | Author logs outside the repository in the session scratch folder `s01t02/run/` (numbered by step); smoke evidence in `.local/dev-stack-smoke/<project>/` (ignored by Git) |
| Static reviews before running | Codex root and the independent reviewer reviewed every script, the Compose file and the tests by exact hash before they ran (snapshot 2, then one stdout delta). All results below are author runs, not reviewer runs. |
| Normal app | Not stopped or restarted. API PID 78497 and Vite PID 78502 ran throughout; paper MD5 `51285359c2d98893a4d47ead4c2fb4da`, 1 research job and its 2 source hashes, identical before and after (read-only queries). `.env.local` unchanged (mode 600, ignored). |

## What exists now

| Deliverable | Content |
|---|---|
| `deploy/compose.yaml` | PostgreSQL 16.15 and NATS 2.15.1 (pinned by digest), platform-core, and one-shot jobs for market-ingestion, research-worker and the NATS volume owner. The project name, ports and secret folder come from `scripts/dev-stack.sh`; the file refuses to render without them. |
| `deploy/Dockerfile`, `.dockerignore` | One image for the three entrypoints: Go built in `golang:1.26.10-trixie@sha256:909771fe…`, the research wheel built with hash-pinned uv, runtime `python:3.14.8-slim@sha256:fbecd461…` as uid 10001. Secrets, `.git`, `.local`, `.tools`, `docs` and fixtures stay out of the build context. |
| `scripts/dev-stack.sh` | The one documented command: `up [--no-seed]`, `refresh-fixtures` (says it changes data), `status`, `logs`, `digest`, `stop`, `down` (keeps volumes), `destroy-volumes` (refuses the default project, needs `TS_DEV_CONFIRM`; never run here). |
| `scripts/dev_stack_api.py` | Loopback-only API client: ignores inherited proxies, follows no redirects, never prints the token. |
| `scripts/dev-stack-smoke.sh` | The full check on a new throwaway project (below). |
| `fixtures/synthetic/` | Invented fixtures with a hash manifest, a generator that reproduces them byte for byte, and `label.py`, the fixture adapter that marks every research result SYNTHETIC. |
| `config/example.env` | The settings (project, ports) with no secret; `config/dev.env` is ignored by Git. |
| platform-core and market-ingestion | Read `TS_LOCAL_DEV_TOKEN_FILE` and `DATABASE_URL_FILE` (Compose secrets). Setting both forms is refused, the file is capped at 4096 bytes, and errors never contain the secret. |

Resource map: Compose project `tradestocker-dev` (smoke and review instances `tradestocker-dev-<suffix>` only); image `tradestocker-dev-app:<project>`; volumes `<project>_pgdata`, `_natsdata`, `_research-results`; networks `<project>_internal` (no route out) and `_edge` (platform-core only); API `127.0.0.1:18280` (smoke `18380`), frontend `15273`; PostgreSQL and NATS not published; secrets `.local/dev-stack/<project>/` (700/600).

## Acceptance criteria

| Criterion | Outcome | Evidence |
|---|---|---|
| One documented command starts dependencies with persistent development volumes and health checks | **Met (author run).** `scripts/dev-stack.sh up` builds, starts PostgreSQL, NATS and platform-core with health checks and waits until all are healthy. Data is in named volumes and survived a real `down` and `up` (new containers) byte for byte. | Smoke steps 1, 5; default project runs |
| Containers run as non-root where feasible; secrets supplied outside tracked files | **Met (author run).** Running processes: platform-core uid 10001, NATS `nobody` (65534), both with a read-only root filesystem and all capabilities dropped; PostgreSQL server processes uid 70 (`postgres`) after the official entrypoint starts as root (it needs default capabilities). Secrets are created once per project with `O_EXCL` in `.local/dev-stack/<project>/` (700/600), passed as Compose secret files, never printed. | `11-runtime-users.txt`; `test_dev_stack.py` |
| Offline fixtures run without provider or LLM credentials | **Met (author run).** No provider, model, SEC contact, Telegram or broker setting exists in the stack. research-worker runs with `network_mode: none`. Stored research results equal the labelled fixture file byte for byte and contain no SEC source, link or permission. | Smoke step 3; `test_fixtures.py` |
| Teardown is non-destructive by default; removing volumes is explicit | **Met (author run).** `stop` and `down` kept every volume (checked after each run). Volume removal exists only in `destroy-volumes`, which refuses the default project and needs `TS_DEV_CONFIRM`; it was tested only against a record-only docker stub and never run for real. | `test_dev_stack.py`; volume listings |
| Validation: smoke-test clean startup, shutdown and restart with persisted fixtures | **Met (author run).** See the smoke below. | `8-smoke-2.log` |
| Validation: no live advisory delivery in development defaults | **Met (author run).** Every summary shows `mode: PAPER`, `live_advisory_enabled: false`, `research_configured: false`. Nothing is configured to send anywhere. platform-core's `edge` network can reach outside, so this rests on configuration, not network isolation. | Smoke summaries |

## Commands and results (author runs, in order)

| Command | Result |
|---|---|
| `python3 -B scripts/test_dev_stack.py` (fake repo, record-only docker and API stubs) | 13 passed (reviewed version `4734a8b4`) |
| `env -u DEV_STACK_API_CLIENT python3 -B scripts/test_dev_stack_api.py` | 4 passed: an inherited hostile proxy (owned by the test) received 0 requests; a redirect to an owned trap was not followed; non-loopback targets refused before any request |
| Red control: the same test against a client without the proxy fix (outside the repo) | Exit 1, 3 observed failures (proxy, seed and redirect tests; the proxy received the requests). Only a synthetic token was used. |
| `make check`, `make test`, `make test-package`, `make arch` | All exit 0. Go: 8 packages. Python: research 35, fixtures 7, launcher helpers 10, package guard 16, toolchain paths 6, dev stack 13, API client 4, import boundary 4 |
| **First smoke**, project `tradestocker-dev-smokeea38b0a1` | **FAILED at step 1.** The stack came up healthy, but `dev-stack.sh up` printed Docker build output on stdout, so the JSON summary check failed. Fixed by sending all Compose output to stderr (statically re-reviewed delta). Its `summary-1.json`, log and volumes are kept as evidence. |
| `python3 -B scripts/test_dev_stack.py` after the fix | 13 passed. This was run once before the delta's static check (disclosed) and once after it. |
| **Second smoke**, `scripts/dev-stack-smoke.sh`, project `tradestocker-dev-smoke1d8563c0` | **PASS**, all five steps (below) |
| Default project: `dev-stack.sh up`, `status`, `digest`, `down` | All exit 0. Seeded a new workspace. |
| Default project restart for runtime inspection, **first attempt** | **Did not run.** `up` and `down` exited 127 (command not found): an `env -u …` prefix stored in a shell variable is not split into words by zsh. No container was started; the `docker inspect` errors in that log only reflect that. |
| Default project restart, second attempt: `up`, `digest`, `down` | All exit 0; digest and summary identical to the first start. The process-user query failed (`docker top -o` needs the `pid` column); read-only root filesystem and dropped capabilities were recorded. |
| Default project restart, third attempt: `up`, process users, `down` | All exit 0; process users recorded (see the criteria table); summary unchanged. |

### The passing smoke, step by step

1. **Clean start, nothing loaded:** `PAPER`, live advisory false, research not configured, no accounts, no quotes.
2. **Real frontend browser tests** against this stack's API (`API_PROXY_TARGET=http://127.0.0.1:18380`, frontend on 15273): 7 passed, 1 skipped (the external SEC test, skipped by design).
3. **Seed fills only what is missing:** it added the EGP fixture account and the research results; the browser tests had already created the USD account and loaded the synthetic quotes. 5 quotes, all `SYNTHETIC`. Stored research results SHA-256 `e886cd1f…` equal `fixtures/synthetic/research/expected-results.jsonl`; 0 SEC source or link strings. No recommendation was close to expiry (a due expiry would be a legitimate write on read).
4. **A second up changes nothing:** paper digest `935087b891ccbf0ce8f1e6cada67e082` and research digest unchanged.
5. **Real down and up:** the platform-core container was recreated; the paper digest, research digest and full summary are identical to step 3.

The default project's digests: paper `0ed776ab004a794a1edce6265e3baf47`, research `e886cd1f…`, unchanged over three starts.

## Limits

- **Secret files on Linux are unchecked.** On Docker Desktop for Mac a 600 file is readable by uid 10001 in the container (tested with a fake secret). On a Linux Docker host a bind mount keeps the host owner, so the app containers would need a matching owner there. CI does not start the stack.
- **NATS runs but nothing uses it yet;** the outbox/inbox work is S02-T04.
- **platform-core is not network-isolated;** see above. research-worker has no network; market-ingestion uses platform-core's loopback.
- **Images are not reproducible byte for byte** (each build had a new image ID); the Go binaries and wheel inside are built the same way as `scripts/package.sh`.
- **Left in place, not deleted:** the volumes of both smoke projects and the default project; the images `tradestocker-dev-app:{buildcheck,tradestocker-dev,tradestocker-dev-smokeea38b0a1,tradestocker-dev-smoke1d8563c0}`; their secret and evidence folders under `.local/`; two fake-secret check folders (one in `.local/`, one outside the repository). Removing any of them is a separate, explicit step.
- Fixture research results are labelled SYNTHETIC and are never paper recommendations or research evidence.
