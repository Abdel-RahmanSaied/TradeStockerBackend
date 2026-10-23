# Synthetic development fixtures

**Everything here is invented.** The issuer "Synthetic Test Issuer (invented)", the symbol `SYN.TEST`, CIK `0000000001`, its filings, its figures and the paper account amounts do not describe any real company, market, filing or account. They exist so the local stack and the tests run offline, without any provider, model or broker credential. No licensed or downloaded dataset is stored in Git.

| File | What it is | Used by |
|---|---|---|
| `paper-accounts.json` | Two paper accounts (USD, EGP) with fixed idempotency keys | `scripts/dev-stack.sh` initial seed (through the API) |
| `research/requests.jsonl` | Three research-worker requests: an SEC-shaped annual-report request for the invented issuer, a stub request, and one request for an unknown task (`place-order`) that must be rejected | the `research-worker` job in `deploy/compose.yaml` |
| `research/expected-results.jsonl` | The exact stored results for those requests: research-worker's output after `label.py` | the stack smoke test and `test_fixtures.py` |
| `label.py` | The fixture adapter: marks each result SYNTHETIC with its fixture file, SHA-256 and line; replaces the SEC source labels and removes filing links, because the filings do not exist; fails closed if any official claim or permission remains | the `research-worker` job in `deploy/compose.yaml` |
| `manifest.json` | SHA-256 of each fixture file | `test_fixtures.py` |
| `generate.py` | Writes the two input files, byte for byte the same every time | regenerating after a deliberate change |

The parser itself always describes SEC filings, because that is what it normally reads. Its raw output is therefore never stored from these fixtures; parser behaviour is tested separately in `research/tests`.

The synthetic market observations (quotes) that `market-ingestion` loads are defined in `internal/paper` (`LoadFixtures`). They are stamped with the load time on purpose, because the paper freshness check needs fresh observations, so they are not a file here.

After a deliberate change: run `python3 -I fixtures/synthetic/generate.py fixtures/synthetic`, regenerate the expected results with the pinned research-worker piped into `label.py`, update `manifest.json`, and check with `make test`. Fixture research results are never paper recommendations and are not shown as research evidence in the dashboard.
