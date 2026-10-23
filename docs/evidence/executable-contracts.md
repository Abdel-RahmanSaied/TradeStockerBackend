# Task evidence — S02-T01

Implement OpenAPI and event-schema ownership.

| Field | Value |
|---|---|
| Task / owner / reviewer | `S02-T01` / `claude-code-agent` (session `2026-10-23-development-8da61cee`) / **independent review pending** |
| Base | Backend `f5d2179a937f5fe72c30ec1fe795ff06da714a5e` (state 74) |
| Code | Recorded by the coordinator from the author's commit; this record is part of it |
| Environment | macOS 27.0.1 arm64; Go 1.26.10; CPython 3.14.8 (research venv); no new third-party dependency |
| Hosted CI | **Not run.** Nothing was pushed. |

Every result here is an **author run** unless it says otherwise. The independent reviewer's runs are in its own reports.

## What exists now

| Part | What it is | Runtime |
|---|---|---|
| `contracts/openapi/local-api.v1.json` | OpenAPI 3.1 of the 19 served routes, every request and answer typed | Served; reconciled with the route registrations in the Go source |
| `contracts/research/sec-annual-v2.{request,report}.schema.json` | The Go → Python parser request and the Python → Go report, existing wire format | Emitted on every research attempt |
| `contracts/events/v1/` | Envelope, shared value types, the 13 `CONTRACTS.md` events | Contract only; no producer yet (S02-T04) |
| `internal/contracts` (Go, standard library) and `tradestocker_research.contracts` (Python, standard library) | The same strict reader, schema compiler, pattern automaton, validator, canonical writer and event check | Used by tests and tools; not yet called by a production consumer |
| `internal/contracts/events_v1.go`, `tradestocker_research/events_v1.py`, `…/contract_documents.py` | Types and embedded documents generated from the schemas | |
| `contracts/embed.go` (`contractdocs.FS`) | The contract documents embedded for Go | |
| `contracts/tools/compat.py`, `compat/v1.lock.json` | Direction-aware compatibility check against a reviewed baseline (185 surfaces) | `make ci-go`, `make test` |
| `contracts/README.md` | Dialect, wire rules, versions, compatibility policy, reconciliation with `CONTRACTS.md` | |

Product behaviour changed in one place only: `internal/research/analyze.go` now builds the parser request in `parserRequest`, with the same bytes as before (the time is passed in). No API, storage, paper, frontend or persisted format changed.

## Acceptance criteria

| Criterion | Outcome (author run) | Evidence |
|---|---|---|
| Decimal values cross service boundaries as strings; timestamps are UTC RFC3339 | **Met for the contracts and payloads checked.** Event and SEC report decimals are bounded strings (30 integer, 18 fraction digits; scale kept, nothing rounded); JSON numbers, exponents and 19-digit fractions are refused. Validators assert UTC RFC 3339 with real calendar dates (`Z` in events and the API, `+00:00` as the SEC report writes it). PAPER API money stays exact integer minor units within ±(2^53 − 1): an existing integer format, documented, not relabelled. **Pending:** the live research handler's PostgreSQL timestamps (`TestLiveResearchAnswersMatchTheContract`, needs a disposable database). | validator, events, research and API corpora; `TestServedAnswersMatchTheContract`; `TestGoBuildsTheParserRequestTheContractDescribes` |
| Envelope contains event/causation/correlation IDs, schema version and producer build | **Met.** All required, non-null except `account_id`; each removed, nulled, emptied, oversized or control-character case is refused in both languages. | `events/v1/envelope.schema.json`; events corpus |
| Unknown enum values and breaking schema changes fail compatibility tests | **Met.** Unknown enums, versions and types are quarantined (`UNSUPPORTED_SCHEMA_VERSION`, `UNKNOWN_EVENT_TYPE`, `SCHEMA_VIOLATION`). The compatibility check diagnoses removed or renamed fields, decimal-to-number, removed enum values, widened safety constants (including `false` → `0`), opened objects, changed reference targets, removed schemas or operations, newly required bodies and parameters, security and response-header changes, schemas that do not compile and malformed locks. | `contracts/tools/test_compat.py` (29 tests) |
| Execution-report endpoints record past human fills; no order-submission endpoint exists | **Met for PAPER.** `…/fill` and `…/sell` record executions a human already made (marked `x-tradestocker-execution-report`); no order, execution or broker path exists in the contract or at runtime (7 such paths answer 404). The actual-account endpoint is not served (needs S01-T04); its contract is the event `portfolio.execution.recorded.v1`. | `TestNoOrderSubmissionEndpointExists`, `TestDocumentedRoutesAreExactlyTheServedRoutes` |
| Validation: cross-language round-trip golden fixtures and invalid-payload tests | **Met.** 13 golden events decode into the generated Go and Python types and encode back to identical bytes in both. The SEC golden report is the parser's exact output through the real launcher, accepted by `checkReport`. 395 hand-written cases (validator 210, events 93, research 53, API 39) give the same expected answer in Go and Python. 8 282 generated inputs give identical verdicts from Go and the installed Python module. 31 API fixtures (23 captured from the real paper handlers, 8 built from the real research types) match the contract in both. | `internal/contracts`, `internal/research`, `internal/httpapi`, `research/tests/test_contracts.py` |

## Runs

| Command | Result |
|---|---|
| `go test -race -count=1 ./...` (no `TEST_DATABASE_URL`, `GOFLAGS` unset, pinned Python) | All packages `ok`; in `internal/contracts`, `internal/httpapi` and `internal/research`: 50 top-level PASS, 16 SKIP (the database tests) |
| `.tools/research-venv/bin/python -I -B -m unittest discover -s research/tests -t research/tests` | 60 PASS |
| `… -m unittest discover -s contracts/tools -t contracts/tools` | 29 PASS |
| `contracts/tools/generate_contracts.py --check` | exit 0 (generated code current; no difference to the lock) |
| ruff, mypy (14 files), `go vet ./...`, gofmt, `planctl validate`, `render --check` | clean / PASS |
| **Not yet run** | `scripts/test-isolated.sh ./internal/research` with the new `TestLiveResearchAnswersMatchTheContract` (static review requested first) |

## Review findings during the task (kept, not rewritten)

- 038: kept the PAPER integer format and the SEC report's string versions; real payloads checked, not only invented fixtures; the `.v1` naming rule written down.
- 039: `positive_decimal` allowed 35 fraction digits; now composed from the bounded non-negative rule.
- 040: malformed keyword shapes were ignored, recursion had no guard, referenced roots were misjudged; now every schema compiles first (shapes, references, cycles, size).
- 041: parser-only independent run passed (json.go `0f9a7afd…`, unchanged since).
- 042: Go and Python regex engines disagreed (`^a|b$`, POSIX classes, `\Q`, back-references) and a 600-document chain exhausted Python's recursion; both now use the dialect's own grammar and a linear automaton, with a 64-level depth limit. Nested repetition is now accepted because it can no longer backtrack (rationale kept in the corpus).
- 043: legacy bounds aligned (symbol length bounded only by the 64 KiB body, `stop_minor` ≥ 1, both SEC CIK encodings in both originals, exact report integer ranges and array caps).
- 044/045: signed counts in Go and cached subtrees bypassing the depth limit; fixed with unsigned digits and subtree heights (independent recheck 046: 13/13 per language).
- 046: compatibility compared `false` and `0` as equal, ignored required bodies and parameters, and normalized without the shared compiler; fixed (independent recheck 047: 10/10 cases).
- 047: an empty lock surface skipped every comparison; exact surface shapes are now validated.
- Found by the new tests themselves: Evaluate adds `versions.assessment` and `versions.observation` when a quote exists (schema extended); an unknown long symbol ends as `NO_TRADE`, not `INSUFFICIENT_DATA`; Go passes the SEC originals re-encoded compactly (value kept), which `contracts/README.md` now states.

## Limits

- No production code validates payloads at run time yet; the v1 events have no producer.
- Schemas do not check relations (issuer identity, fact arithmetic, past execution time, ownership).
- Go's automatic `HEAD` handling and router 404/405 answers are not separate operations.
- No stress or adversarial resource test was run; the limits (bytes, values, depth, states, steps) are implemented and unit-tested at their edges only.
- Frontend: unaffected (no served answer changed); its browser suite was not rerun for this task.
