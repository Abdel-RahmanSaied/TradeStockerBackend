# research-worker protocol (local)

The Python research entrypoint: `research-worker` (console script of the `tradestocker-research` package) or `python -I -m tradestocker_research.worker`. It takes no arguments.

## Input and output

One JSON request per line on stdin, one JSON result per line on stdout, in the same order. Blank lines are skipped. A line may be at most 40 MiB + 4 KiB.

```json
{"task": "sec-annual-v2", "input": { ... }}
```

A request may also carry `"traceparent"` (W3C, version 00). Each request then logs one JSON line on stderr in that trace (a new span), or in a new trace if it is missing or malformed. The line has only the task name (or `unknown`), `ok` or `REJECTED`, the error type name, the input and output sizes and the duration; never the input, document content or error text.

| Task | Input | Result |
|---|---|---|
| `sec-annual-v2` | The SEC parser request: `symbol`, `cik`, `as_of`, `submissions`, `companyfacts`, `submissions_sha256`, `facts_sha256` | The same report the parser process writes (byte-identical encoding) |
| `research-stub-v1` | `{"symbol": "...", "mode": "PAPER"}` | A stub with `outcome: INSUFFICIENT_DATA` and no evidence |

A bad line (not JSON, wrong envelope, unknown task, invalid input, over the limit) gives one result `{"status": "REJECTED", "error": "..."}` and the stream continues.

## Boundaries

- Results never make anything eligible for entry or backtesting and never claim financial-state write access. Go checks every stored SEC report again (`checkReport`).
- The package code opens no network, database or file connection and is given no credentials. This is a code boundary, not a sandbox: the CLI runs with the caller's OS permissions.
- platform-core does not call this CLI yet. It runs the same parser source (embedded from `financials.py`, byte for byte) as an isolated subprocess with `-I` and only `PATH` and `LANG`, through `research/parser_launcher.py`: the launcher loads the parser and `telemetry.py` from their embedded sources, calls the parser's `main()` (so stdout is exactly the parser's report) and writes one `research parser ended` event on stderr in the trace Go passes as its only argument. Go relays that event to its log only if it has exactly the event's fields, the job's trace ID and short names; any other stderr (which can quote the input) is logged as a byte count only. A queue between platform-core and research-worker is later work (S02-T04).
