# Independent compatibility review

Reviewer: `independent-review-agent`, separate from implementation owner `claude-code-agent`. Review date: 2026-10-23. This reviews the bounded technical part of S00-T05, not production readiness or completion of the original task.

## Exact scope and decision

The independent runtime run used a complete `git archive` of backend `8d7d112fb3e5abd8395de14dbdbf66df31daf58f`, with separate tools, caches and outputs outside the development checkout. Frontend `2e1841916eecefbb954a3c304d965cff3cf93744` was unchanged and outside this review. The subsequent diff to `dd2f4b410d782562f79714245ae1db6e666d3cc4` changes only comments and evidence/README wording in four files. The final correction `1e778e4497fbcd2bb70cfc06cf9c820d5694395a` changes only the compatibility matrix. Intervening coordinator commits affect planning documents/state only. No executable statement, dependency, configuration or test changed between the runtime baseline and the accepted checkpoint. The runtime evidence therefore also applies to the corrected checkpoint.

**Technical subset accepted at 1e778e4497fbcd2bb70cfc06cf9c820d5694395a:** pinned Go/Python clients build and connect to real disposable PostgreSQL and NATS JetStream; duplicate publication and consumer redelivery preserve the tested inbox effect; the selected LangGraph PostgreSQL checkpointer persists a pause and resumes in a different process; the local load/storage measurements are reproducible. Both findings below are resolved. This is not acceptance of S00-T05 as DONE: D-05/D-06/D-07 and the documented workload, retention and budget inputs remain open.

## Findings and resolutions

1. **Producer outbox guarantees were overstated — resolved in dd2f4b4.** At 8d7d112, `spikes/s00-t05/go/main.go:92` claimed business state and outbox rows committed together, and `docs/evidence/compatibility-matrix.md:39` reported that as PASS. The producer only bulk-inserts outbox rows; it publishes from memory and marks all rows afterward. Neither atomic business-plus-outbox writes nor a restarted durable relay was tested. The corrected comments, README and evidence now describe the actual compatibility test and its limits. No production outbox guarantee is accepted.
2. **Python lifecycle date was inaccurate — resolved in 1e778e4.** `docs/evidence/compatibility-matrix.md:16` at dd2f4b4 said Python 3.14 has bugfix support until October 2030. The official schedule gives final regular bugfix binaries in October 2027, followed by source-only security fixes until October 2030. The final docs-only commit corrects the support cell and links the schedule. The selected 3.14.8 interpreter is currently supported, so this did not invalidate the runtime compatibility result. Source checked during review: [PEP 745](https://peps.python.org/pep-0745/).

No further blocking correctness or isolation defect was found in the bounded spike. A successful controlled pause/resume does not prove exactly-once external side effects under arbitrary process crashes: the node counter and graph checkpoint are separate database transactions.

## Independent commands and results

The archive was created with `git archive 8d7d112fb3e5abd8395de14dbdbf66df31daf58f`. From that archive, the following complete run returned exit 0:

```sh
SPIKE_SAMPLE_DIR=/Users/asaied/Documents/Codex/2026-10-23/referenced-chatgpt-conversation-this-is-an-4/work/sec ./spikes/s00-t05/run.sh
```

The supplied SEC files were already cached. No SEC or model request was made. Normal application credentials and database were not used by the spike. The run independently downloaded its toolchains and locked packages into the archive's own `.tools`; existing official container images were allowed to be reused. Summary and phase JSON were retained in external reviewer scratch, not substituted for author measurements.

| Check | Independent result |
|---|---|
| Actual Go / pgx / nats.go | go1.26.10 / v5.11.0 / v1.54.0 |
| Actual PostgreSQL / NATS server | 16.15 / 2.15.1 |
| Actual Python | CPython 3.14.8; exact patch pin checked by the result validator |
| Actual Python packages | langgraph 1.2.14; langgraph-checkpoint 4.2.0; langgraph-checkpoint-postgres 3.1.2; psycopg and psycopg-binary 3.3.6; psycopg-pool 3.3.3; nats-py 2.16.0; certifi 2026.10.19 |
| Go publication | 5,000 originals; 500 repeats, all flagged as duplicates; exactly 5,000 stored messages / 720,000 bytes |
| Go inbox effects | 384 deliberately skipped acknowledgements and 384 redeliveries; 5,000 inbox rows; effect total 19,995, equal to expected total |
| Python JetStream | first publish not duplicate; second duplicate; one delivered message; no extra message after acknowledgement |
| Checkpoint across processes | PID 2246 paused after collect/assess, with report pending; PID 2247 resumed to completion; durable counts collect=1, assess=1, report=1; different pause/resume checkpoint IDs |
| Python advisory scan | OSV queried 45 locked registry packages; no advisories returned |
| Sample source storage | 3,789,099 company-facts bytes + 163,917 submissions bytes; PostgreSQL table 671,744 bytes |
| Cleanup | no remaining spike containers or listeners on 55532/54222/58222/18180 |

The eight-client, five-second load phases all had **zero errors**:

| Phase | Requests | Requests/s | p50 / p95 / p99 (ms) |
|---|---:|---:|---|
| Dashboard, small state | 9,236 | 1,845.6 | 1.647 / 12.775 / 61.592 |
| Candidate evaluations | 395 | 76.5 | 102.143 / 176.567 / 248.443 |
| Dashboard after evaluations | 212 | 40.8 | 194.848 / 204.988 / 207.346 |

The resulting disposable state contained exactly 395 recommendations and 397 audit events. Its JSON document was 1,120,884 bytes and table 91,840,512 bytes. These independently reproduce the large-history read bottleneck; they do not set production targets. The evaluations are paper API operations, not executed trades or a count of approved entries.

Seven external negative-input checks exercised the result validator without services or repository edits. Each returned exit 1 with a failed result: a nonzero load error count, zero load requests, reused checkpoint PID, a node executed twice, an extra Python message, a failed Go phase, and an OSV advisory. The Go implementation also explicitly checks inbox count, expected effect total, stored message count and duplicate/redelivery counts.

After cleanup, the normal authenticated API reported `status=ok`, `mode=PAPER`, `storage=postgresql`, `live_advisory_enabled=false`; the normal frontend returned HTTP 200. A read-only SQL check returned normal `paper_workspace` document MD5 `51285359c2d98893a4d47ead4c2fb4da`, unchanged from the coordinated pre-run value. Only the normal PostgreSQL container remained running. No normal resource was stopped or removed.

## Pinning, support and remaining limits

Reviewed `go.mod`/`go.sum`, the hash-bearing `uv.lock`, exact `.python-version`, hash-required uv installation, and both Compose image index digests. The interpreter and package versions above came from the actual run, not merely the declared pins. The lock records the 2026-10-09 dependency cutoff plus the single certifi exception. That exception is justified by the maintainer's [compromised-root advisory](https://github.com/certifi/python-certifi/security/advisories/GHSA-6q48-6c7w-9gw2); the chosen version is newer than the advisory's patched release. The independent OSV result is dated evidence, not a guarantee against undisclosed vulnerabilities. The author's separate govulncheck result was inspected as evidence, not rerun or claimed as an independent execution here.

Official sources independently checked include [Go's support policy](https://go.dev/doc/devel/release), [PostgreSQL's version/support table](https://www.postgresql.org/support/versioning/), [NATS server v2.15.1 release](https://github.com/nats-io/nats-server/releases/tag/v2.15.1), and [Python 3.14's release schedule](https://peps.python.org/pep-0745/). The chosen supported versions need not be the newest major versions. The product's existing Python 3.9 runtime and older PostgreSQL image build were not changed by this spike.

The measurements use one local machine, one synchronous Go publisher, one consumer, eight concurrent paper clients and one cached Apple source sample. They do not establish multi-process relay recovery, sustained soak performance, production durability/high availability, vendor feed capacity, issuer-size distribution, hosting prices, model costs, licensing conclusions, or production authentication. S3/Parquet, sqlc/goose and other planned integrations were not exercised. No paid resource, external model, broker action, Telegram send, production deployment, live activation or risk-policy change occurred.

Repository planning checks also passed during review: `planctl.py validate`, `status`, `ready` and `render --check`. Those checks establish planning consistency only. The coordinator retains sole responsibility for state transitions and recording the unresolved original S00-T05 decisions.
