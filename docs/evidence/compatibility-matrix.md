# Compatibility matrix — S00-T05 (local spike)

Owner: `claude-code-agent`. Checked 2026-10-23 (official sources fetched 06:35 UTC). Spike code: `spikes/s00-t05/` (separate Go module, its own Compose project, tools and caches in `spikes/s00-t05/.tools/`). The product's runtime dependencies were **not** changed.

"Verified" means the spike ran it on this Mac (macOS 27.0.1, arm64, Docker 29.7.2) and checked the result; see the [load and cost profile](load-and-cost-profile.md) for numbers.

## Versions

| Component | In the product today | Spike pin (verified) | Newest stable on 2026-10-23 | Support status (official) |
|---|---|---|---|---|
| Go | `go 1.26.10` in `go.mod` (system install is 1.26.6; Go fetches 1.26.10 automatically) | go1.26.10 | go1.27.3 (go1.27.0 released 2026-08-19) | "Each major Go release is supported until there are two newer major releases": 1.26 stays supported until Go 1.28 |
| pgx | v5.11.0 | v5.11.0 | v5.11.0 (newest tag) | — |
| PostgreSQL | 16.15 (`postgres:16-alpine` build of 2026-08-13, digest `cf78e766…`) | 16.15 (`16.15-alpine` index digest `f4610795…`, 2026-10-22 build) | 18.6 (newest major); 16.15 newest 16.x | 16 supported until 2028-11-09; 18 until 2030-11-14 |
| NATS server | not used | 2.15.1 (`nats:2.15.1-alpine` index digest `7ae34c25…`) | v2.15.1 (2026-10-09) | — |
| nats.go | not used | v1.54.0 | v1.54.0 (2026-09-18) | — |
| Python | system `/usr/bin/python3` 3.9.6 runs the research parser | CPython 3.14.8, uv-managed, inside `.tools/` | 3.15 (first release 2026-10-09) | **3.9 end-of-life since 2025-10-31**; 3.14: regular bugfix releases with binary installers until 3.14.14 (2027-10-05), then source-only security fixes until about 2030-10 (PEP 745); 3.13: security fixes until about 2029-10 (PEP 719) |
| uv | not installed | 0.13.0 (pip, hash-pinned) | 0.13.5 (2026-10-22) | — |
| LangGraph | not used | langgraph 1.2.14, langgraph-checkpoint 4.2.0, langgraph-checkpoint-postgres 3.1.2 | 1.2.16 / 3.2.0 | All require Python ≥ 3.10 |
| psycopg | not used | psycopg 3.3.6 (+ binary), psycopg-pool 3.3.3 | same | Require Python ≥ 3.10 |
| nats-py | not used | 2.16.0 | same | Python ≥ 3.7 |
| Node.js (frontend) | v26.7.0 | not part of the spike | v26.12.1 (not LTS in the official index); v24.22.1 "Krypton" is LTS | — |

Sources: <https://peps.python.org/pep-0745/>, <https://peps.python.org/pep-0719/>, `go.dev/dl/?mode=json`, <https://go.dev/doc/devel/release>, `peps.python.org/api/release-cycle.json`, `nodejs.org/dist/index.json`, <https://www.postgresql.org/support/versioning/>, PyPI JSON API, GitHub releases API (nats-io/nats-server, nats-io/nats.go, jackc/pgx), Docker Hub tags API.

## Pinning used in the spike

- Python interpreter: exactly CPython **3.14.8**, set in `spikes/s00-t05/python/.python-version`, installed by uv into `.tools/` with no shims in `~/.local/bin`. The results check fails if any other version runs.
- Go: `spikes/s00-t05/go/go.mod` and `go.sum` (checksums from the Go module proxy and checksum database).
- Python: `spikes/s00-t05/python/uv.lock` with file hashes. A **14-day cooldown** (`exclude-newer = 2026-10-09`) keeps very new releases out. One security exception: certifi 2026.10.19, because older certifi bundles emSign root CAs with compromised intermediate keys (GHSA-6q48-6c7w-9gw2); a constraint requires at least that version.
- uv itself: `spikes/s00-t05/tools-requirements.txt`, installed with `pip --require-hashes` using hashes from PyPI.
- Containers: Docker Hub **index digests** for `postgres:16.15-alpine` and `nats:2.15.1-alpine`.

## What was verified

Final evidence run: 2026-10-23, 06:53–06:54 UTC, from a **clean state**. The spike's `.tools` folder (Go module cache removed with `go clean -modcache`) and both spike images were deleted first. In 78 seconds, `run.sh` downloaded the go1.26.10 toolchain, pulled both images by digest, installed uv (hash-checked), Python 3.14.8 and every locked package, and passed all checks below.

| Check | Result |
|---|---|
| Go + pgx bulk-load outbox rows (COPY) on PostgreSQL 16.15 | PASS. Not proven: atomic business-plus-outbox commits or a durable relay that recovers after a restart; events are published from memory |
| Go + nats.go publish to JetStream with message ids; a repeat of 500 ids | all 500 flagged as duplicates; the stream kept exactly 5,000 |
| Consumer applies effects through an inbox, then acks; 384 acks skipped on purpose | 384 redeliveries, 5,000 inbox rows, exact effect total (no double effect) |
| Python nats-py against the same server | repeat id flagged; exactly one message stored and delivered |
| LangGraph PostgreSQL checkpointer: pause before the last node, exit, resume in a **new process** | finished from the checkpoint; each node ran exactly once |
| Product `platform-core` (unchanged code) on PostgreSQL 16.15 at the newest digest | migrations ran; load profile completed with 0 errors |

## Vulnerability checks

- `govulncheck` on the spike module: "Your code is affected by 0 vulnerabilities." One module-level notice, GO-2026-5932: `golang.org/x/crypto/openpgp` is unmaintained and unsafe by design (no fix version). The spike does not import `openpgp`; `x/crypto` comes in through NATS key handling. The product module does not depend on `x/crypto` today.
- OSV batch query for all 45 locked Python packages: 0 advisories after the certifi exception. `run.sh` repeats this check on every run and fails on any advisory.
- Two mistakes were caught by these checks before this record: `UV_NO_CONFIG=1` had silently disabled the cooldown (the first lock held packages from 2026-10-22), and the cooldown then picked a certifi with a known advisory. Both are fixed, and the lock now records the cutoff and the single exception.

## Findings for later tasks (not changed here)

1. **The product's research parser runs on Python 3.9, which is end-of-life.** The planned Python stack (LangGraph, psycopg 3.3) cannot run on 3.9 at all. The spike shows CPython 3.14.8 under uv works with every planned library. Moving the product is a separate, reviewed change.
2. The product's PostgreSQL pin is an older build of the same 16.15 version. Updating to the current index digest is a small infrastructure change to coordinate.
3. The frontend runs Node 26, which the official index did not list as LTS on 2026-10-23; v24 is the LTS line. Production should choose an LTS line (D-06).

## Not decided here

Identity provider (D-05), deployment and hosting (D-06), and performance/cost targets (D-07) remain OPEN owner decisions. Nothing in this matrix selects them.
