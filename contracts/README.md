# Contracts

The executable contracts between TradeStocker's parts (S02-T01). `docs/CONTRACTS.md` is the planning baseline; what is here, with its tests, is the contract (`docs/SOURCE_OF_TRUTH.md` §4). A contract here is not proof that a feature runs: the table below says which ones are served or emitted today.

| Folder | What | Runtime today |
|---|---|---|
| `openapi/local-api.v1.json` | OpenAPI 3.1 of the local API platform-core serves: PAPER workspace, SEC research jobs, health | **Served.** All 19 routes, and only those (a test reads the route registrations in the Go source). Real answers are validated against it in tests. |
| `research/sec-annual-v2.request.schema.json`, `…report.schema.json` | The SEC parser's stdin (Go → Python) and report (Python → Go) | **Emitted** on every research attempt. Go's request and the actual parser path are checked against them; the report golden is the parser's exact output. |
| `events/v1/` | The shared event envelope, shared value types and the 13 events of the `CONTRACTS.md` catalog | **Contract only.** No producer emits these yet; the outbox (S02-T04) will. Each event says so in `x-tradestocker-runtime`. |
| `fixtures/` | Golden payloads and hand-written conformance cases | Tests in Go and Python |
| `compat/v1.lock.json` | The reviewed compatibility baseline | Checked by `make ci-go` and `make test` |
| `tools/` | `generate_contracts.py` (generated code and the lock), `compat.py` (compatibility check) and its tests | |

The NATS message platform-core publishes today (`tradestocker.research.job.created`, internal/natsbus) is **not** a v1 event: it is a best-effort wake-up hint whose payload is a bare research job ID, with the request's W3C `traceparent` as a header. It is described in `contracts/research-api.md` and `internal/natsbus`, not here.

## Names and versions

- An event's catalog name (`research.assessment.completed.v1` in `CONTRACTS.md`) is its `event_type` (`research.assessment.completed`, never with the suffix) plus `.v` plus its integer `schema_version` (1). The schema file is the catalog name: `events/v1/research.assessment.completed.v1.schema.json`. An `event_type` with the `.v1` suffix is an unknown type.
- The SEC report keeps its own, older version identity exactly as it is emitted: `version: "sec-annual-v2"` and `schema_version: "2"` (strings). It is a service payload, not a v1 event, and is not reinterpreted.
- The local API is `/api/v1`.

## Wire rules

- **Decimals** (events and the SEC report) are JSON strings: optional minus, no exponent, no leading zeros, at most 30 integer and 18 fraction digits. Trailing zeros are kept as written (`1.50` stays `1.50`); nothing is rounded. A JSON number where a decimal is expected is refused. `positive_decimal` must contain a non-zero digit.
- **PAPER money and quantities** in the local API are exact JSON integers in minor units (cents, piastres) and whole shares, within ±(2^53 − 1). They are not decimals: the paper domain has no fractions of a minor unit. This is the existing, deliberate local PAPER format (`contracts/local-paper-api.md`); it is unchanged and not relabelled as decimal.
- **Timestamps** are RFC 3339 in UTC with a real calendar date, hours 00–23, no leap second and 1 to 9 fraction digits. Events and the API write `Z`; the SEC report writes `+00:00` (Python `isoformat`); `-00:00` and other offsets are refused. The PostgreSQL driver reads times in the server's local zone, so the research API converts each one to UTC before serving it (stored rows are not changed). Dates are `YYYY-MM-DD`. Validators enforce this; `format` is never only an annotation here.
- **Identifiers** in events are opaque ASCII tokens, never free text or a ticker.
- Integers in every schema have both bounds, inside ±(2^53 − 1).

## The schema dialect

`tradestocker-schema-subset/1`: a strict subset of JSON Schema 2020-12, implemented the same way by Go (`internal/contracts`) and Python (`tradestocker_research.contracts`, standard library only, embedded documents, no file access).

- Keywords: `$schema` and `$id` (document roots only), `$ref` (local documents only: an absolute URI, an unknown file or a bad pointer fails), `$defs`, `allOf`, `properties`, `additionalProperties`, `items`, `type` (`object`, `array`, `string`, `integer`, `boolean`, `null`; no `number`), `enum`, `const`, `required`, `minLength`/`maxLength`, `pattern`, `format` (`date-time`, `date`), `minimum`/`maximum`, `minItems`/`maxItems`, plus annotations and `x-tradestocker-*`. Any other keyword makes the schema fail to compile.
- Every schema is compiled before any payload is checked: keyword shapes, references, patterns, no reference cycle, at most 64 levels through subschemas and references (in any compile order) and at most 100 000 expanded nodes. A schema that does not compile validates nothing.
- **Patterns** use the dialect's own grammar and a linear-time automaton in both languages (`internal/contracts/pattern.go`): `^body$` matched against the whole string; no top-level `|`; ASCII literals, a fixed escape list, classes with ranges (no negation or POSIX classes), plain groups, `? * + {m} {m,n}` up to 1000; at most 1000 characters and 10 000 states.
- **JSON input** is read strictly: valid UTF-8, no duplicate keys, no unpaired surrogates, no NaN, nothing after the value, at most 64 levels, 48 MiB, 4 000 000 values and 128 characters per number. Numbers keep their exact text; `1.0` and `1e0` are not integers; `true` is never a number.
- The first violation (keyword and JSON Pointer) is the same in both languages: `$ref`, `allOf`, `type`, `const`, `enum`, string rules (`minLength`, `maxLength`, `pattern`, `format`), integer bounds, then for objects missing required members in listed order, unknown members sorted, known members sorted, then arrays.
- Authoring rules per document kind (`CheckDialect`): events require every property (null for "does not apply"), close every object and give every string a pattern, enum, const, format or reference; service and API schemas bound free text with `maxLength`; `additionalProperties: true` is allowed only on an external original (`x-tradestocker-external-original`, the stored SEC documents).

An event consumer quarantines a message instead of guessing: `MALFORMED_JSON`, `UNSUPPORTED_SCHEMA_VERSION` (an integer other than 1), `UNKNOWN_EVENT_TYPE`, or `SCHEMA_VIOLATION` with the first violation.

## Generated types

`tools/generate_contracts.py` writes, from the event schemas: Go structs and a typed decoder (`internal/contracts/events_v1.go`), Python frozen dataclasses (`research/src/tradestocker_research/events_v1.py`) and the embedded documents (`…/contract_documents.py`). Decimals and timestamps stay strings, so a round trip never reformats a digit. `--check` fails if any is stale. Never edit them by hand. Go code can also use the embedded documents through the asset package `tradestocker/contracts` (`contractdocs.FS`).

## Compatibility

`tools/compat.py` compiles every surface with the shared strict validator, then compares its validation structure (references followed, documentation dropped, exact values) with `compat/v1.lock.json`:

- **Events and service payloads** are strict: any structural difference breaks. Publish a new version (`events/v2/…`) instead of changing v1.
- **API requests** (security, bodies, parameters): a change breaks if the server would refuse what an old client sends, including a body or parameter becoming required or a new required parameter on an existing operation.
- **API responses** (bodies and headers): a change breaks if an old client could receive something the old contract excluded, such as a new enum value or member.

A removed surface, a schema that does not compile and a malformed lock always break. A new surface is reported until locked. `generate_contracts.py --lock` adds new surfaces and accepts non-breaking differences after review; it refuses any breaking one. Documentation-only edits are not differences.

## Fixtures and cross-language tests

- `fixtures/events/v1/`: one canonical golden event per type. Go and Python decode each into their generated types and must encode it back to exactly the same bytes.
- `fixtures/research/`: a synthetic SEC request and the exact report the parser writes for it (Go runs it through the real launcher; Python through `financials.assess`).
- `fixtures/api/v1/`: answers captured from the real paper handlers (and research payloads built from the real Go types), named `operationId.status[.label].json`.
- `fixtures/conformance/*.json`: hand-written cases (validator, events, research, API) with expected answers. Go and Python must both give every expected answer; a Go test also sends thousands of generated inputs to the installed Python module and requires identical verdicts. Cases whose expectation changed with the engine carry a `rationale` with the earlier failure.

Canonical form (both languages): sorted keys, no spaces, integers in plain form, strings escaped only where JSON requires it.

## Reconciliation with `docs/CONTRACTS.md`

| `CONTRACTS.md` | Actual |
|---|---|
| `GET /health/live`, `/health/ready` | Served as `GET /api/v1/live` and `/api/v1/ready` (S01-T05), plus the older `GET /api/v1/health`. |
| `POST /research-jobs`, `GET /research-jobs/{id}` | Served as `/api/v1/research/jobs` and `/api/v1/research/jobs/{id}` (SEC filings only), plus list, sources and status. A research replay returns the job's current state; a paper replay returns the first committed answer. |
| `POST /accounts/{id}/execution-reports` | **Not served** (needs production identity S01-T04 and actual accounts). PAPER execution reports exist: `POST /api/v1/paper/recommendations/{id}/fill` and `/api/v1/paper/accounts/{id}/sell` record executions a human already made. The contract for actual ones is the event `portfolio.execution.recorded.v1`. |
| `POST /recommendations/{id}/responses`, goals, availability, imports, reconciliations, instruments, market status, Telegram webhook | **Not served.** Paper acknowledge/cancel and entry pause cover the PAPER workflow only. |
| `POST /controls/entry-pause` | PAPER only: `POST /api/v1/paper/entries`. |
| No `/orders`, `/execute-trade` or broker tool | **Holds.** No order, execution or broker path exists in the contract or at runtime (tested). |
| Event envelope with `event_type` including no version | Adopted; the catalog's `.v1` names map as described above. |
| 13 catalog events | All 13 are v1 schemas; none is emitted yet. |
| Decimal strings, explicit precision | Events and the SEC report: yes. PAPER API: exact integer minor units (see Wire rules). |
| Error families (`STALE_MARKET_DATA`, …) | The served API uses its existing codes (`STALE_DATA`, `VALIDATION_FAILED`, research codes); error bodies are `{"error": {"code", "message"}}`. The proposed families are not implemented. |

## Limits

- Schemas check shapes and values, not relations: issuer identity, arithmetic between facts, that a reported execution is in the past, account ownership and producer identity are run-time rules.
- Handlers do not validate their own answers at run time; tests do.
- Go answers `HEAD` for `GET` routes and its router writes 404/405 answers; these are not separate operations in the OpenAPI document.
- Go passes the stored SEC originals to the parser with the same JSON value but re-encoded compactly (`<`, `>` and `&` escaped); the request's SHA-256 fields name the stored original bytes.
- The owner token is a single-owner development stand-in, not production identity.

## Changing a contract

1. Edit the schema (a new version for any breaking event or service change).
2. `python3 -I -B contracts/tools/generate_contracts.py` (generated code), then add or adjust fixtures and conformance cases with hand-written expectations.
3. Run the Go and Python tests; `generate_contracts.py --check` shows every difference against the lock.
4. After review, `generate_contracts.py --lock` for new surfaces or non-breaking differences.
