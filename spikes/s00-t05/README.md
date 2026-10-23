# S00-T05 compatibility spike

A throwaway check that the planned stack works together on pinned versions, plus a local load profile. It is **not** product code and does not change the product's dependencies. Results: `docs/evidence/compatibility-matrix.md` and `docs/evidence/load-and-cost-profile.md`.

## Run

```sh
spikes/s00-t05/run.sh
```

Optional: `SPIKE_SAMPLE_DIR=<folder with submissions.json and companyfacts.json>` also measures how much PostgreSQL stores for one research job's original SEC sources.

The script stops at once if any of its ports (55532, 54222, 58222, 18180) is in use. It prints one JSON summary on stdout and exits 1 if any check fails.

## What it checks

1. Go (pgx, nats.go): JetStream publish with message ids, where repeated ids (a simulated relay retry) must be flagged as duplicates; and an inbox consumer with deliberately skipped acks that must apply every effect exactly once. Not proven here: that a producer commits business rows and outbox rows atomically, or that a durable relay reading the outbox table recovers after a restart.
2. Python (nats-py): the same de-duplication from Python.
3. LangGraph with the PostgreSQL checkpointer: a deterministic three-node graph pauses before its last node in one process and finishes in a **new** process; every node must run exactly once. No model is called.
4. The unchanged product API on its own database: read and write latency as the paper state grows.
5. OSV: every locked Python package must have no known advisory.

## Isolation

- Tools, caches and the Python environment live in `.tools/`; outputs in `.local/` (both ignored by Git). Go uses its own `GOMODCACHE`; uv uses its own cache and interpreter folder and installs no shims in `~/.local/bin`.
- PostgreSQL and NATS run in the Compose project `tradestocker-spike-s00t05` with data in memory; `docker compose down` removes everything but the images.
- The normal database (55432), API (8080), dashboard (5173) and `.env.local` token are never used. The product API instance gets its own per-run token.

## Pins

Go module (`go/go.mod`, `go.sum`), Python lock (`python/uv.lock`, 14-day cooldown with one documented security exception for certifi), exact interpreter (`python/.python-version`), uv (`tools-requirements.txt`, hash-checked), and image digests (`compose.yaml`).

To remove everything: `go clean -modcache` with `GOMODCACHE=spikes/s00-t05/.tools/gomodcache`, then delete `.tools/` and `.local/`, and remove the two images by digest.
