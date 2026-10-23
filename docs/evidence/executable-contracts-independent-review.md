# Independent review — executable contracts (S02-T01)

Reviewer: `independent_review`, separate from implementation owner `claude-code-agent`.
Date: 2026-10-23.

## Decision and exact scope

**Accept the bounded S02-T01 implementation at backend `46fc10d7432027ebc20eecc6f5a50a9c9c0bdf4f`, tree `ebe8368c16022d68137afd02afe2bbb1efb8f162`.** No remaining implementation finding was observed in the reviewed scope. The two findings at the original checkpoint are fixed and independently verified. The coordinator owns task-state changes; this report does not mark anything DONE.

Frontend remains `9c75c9183df2ed9457012260bb904a9ea404a960`. The browser review used a private checkout of that commit with one explicitly declared, untracked reviewer test. No frontend product change is part of this task.

Review followed the original catalog acceptance criteria, `docs/CONTRACTS.md`, the source-of-truth/playbook and applicable agent instructions. This accepts executable contracts, shared validation/types and compatibility checks. It does not assert that the 13 catalog events have runtime producers, that an outbox exists, or that future actual-account routes, production identity or live trading are implemented.

The author evidence needs one documentation-only clarification before final closeout: its introductory claim that only parser-request extraction changed behavior predates the UTC fix. It should also name the research API's UTC-Z spelling change, with unchanged instants. Author-only pending/not-run statements must not become claims that the author ran the independent tests. A later documentation commit needs a source-diff/history recheck, not repeated successful runtime suites.

## Acceptance criteria mapped to independent evidence

| Original criterion | Independent evidence and boundary |
|---|---|
| Decimal strings at service boundaries; UTC RFC3339 timestamps | Event/SEC decimal strings preserve scale without float conversion or rounding and reject numeric/exponent/oversized forms. Strict Go/Python parsing, calendar checks and shared invalid corpora passed. The first PostgreSQL response initially failed UTC-Z validation; the exact46fc replay now completes all create/replay/retry/cooldown/status/list/detail/source checks. Two focused UTC tests preserve equal instants, including legacy event `detail.until`. Real API plus browser verification checks persisted synthetic job/event timestamps. The explicitly versioned legacy PAPER API retains exact integer minor units/whole shares within ±(2^53−1); it was not silently migrated or represented as decimal text. |
| Envelope IDs, schema version and producer build | All 13 event schemas compose the required envelope. Shared corpora test missing/null/invalid identifiers, version/type dispatch and unknown values. Generated Go/Python types round-trip every golden event exactly. Catalog subject `.v1` is documented as unversioned `event_type` plus integer `schema_version: 1`; the existing SEC report keeps its string version identities. |
| Unknown enums and breaking schema changes fail compatibility | Both validators enforce expected invalid cases. The direction-aware compatibility suite passed 29 methods; independent earlier probes reproduced and then verified fixes for bool-versus-number equality, required input changes, malformed schemas and malformed lock shapes. Generator preflight and bounded reference walking now pass eight additional focused regressions. No remote reference fetching is permitted. |
| Execution reports record past human fills; no order-submission endpoint | Six targeted HTTP contract tests reconcile all 19 served routes, validate real handler responses and API fixtures, and prove prohibited order/broker routes absent. PAPER fill/sell remain human execution-report recording paths. The proposed actual-account execution-report route is explicitly not served; its event schema conveys no authority to execute. Schemas do not independently prove execution time or ownership. |
| Cross-language golden round trips and invalid payloads | Thirteen golden events round-trip through generated types in each language. The actual embedded parser path produces the exact financial-report golden; Go constructs the real parser request. Both languages check 395 handwritten cases (210 validator, 93 event, 53 research, 39 API). Go and the privately installed Python package agree on 8,282 generated verdict/canonical comparisons, including 1,001 valid inputs. Installed-wheel import/CLI and malformed-then-valid stream recovery passed. |

## Actual execution, separated by checkpoint

All test/resource drivers and added probes were read and hash-cleared by the coordinator before their execution. Author-run counts are not counted as independent results.

| Checkpoint and commands | Actual independent result |
|---|---|
| `2b014911277b6e317af5d2e45c18c3187c422568`, tree `575600b6b3359099620238264ceda1dc68d569f3`: `go test -mod=readonly -race -count=1 -timeout=180s -json` on contracts and anchored new HTTP/research contract tests | Contracts 10 top-level PASS; HTTP 6 PASS; non-DB research 6 PASS. All 22 top-level outcomes PASS, 0 FAIL, 0 SKIP. Nested corpus/differential cases are not added as top-level tests. |
| Same2b: privately installed package `test_contracts.py`; `test_compat.py`; generator `--check`; focused `go vet` | Python 17 PASS and compatibility 29 PASS, no skips/failures. Valid generator check and vet exited0. These successes were reused after byte-level source matching; they were not rerun for the Go serialization fix. |
| Same2b: anchored `TestLiveResearchAnswersMatchTheContract` through reviewed `scripts/test-isolated.sh` | **0 PASS /1 FAIL /0 SKIP** at first POST202 `/created_at`: local-offset spelling `2026-10-23T18:19:18.739373+03:00` broke UTC-Z. Later assertions were not reached. Original log and changes-required report remain intact. |
| `dfeeaedfc62625eecfbf2ad4d42a834723ade4b4`, tree `51c99c037022ac071fcaeb243971e66bb3e35ec1`: `test_generate.py`, then valid `generate_contracts.py --check` | **8 PASS /0 FAIL /0 SKIP**; consistency check exit0. Tests use bounded in-memory cycles/resolver sentinels. No old unbounded generator loop was executed. |
| Exact46fc: anchored `TestResearchTimesFromPostgreSQLAreServedInUTC` and `TestStoredEventDetailIsServedWithAUTCUntil`, race; package vet | **2 PASS /0 FAIL /0 SKIP**, vet exit0. |
| Exact46fc: the **same** anchored real-PostgreSQL test | **1 PASS /0 FAIL /0 SKIP**; attempts1/2/3, replay, retry/cooldown, stored source and all later assertions now complete. This is a successful replay of the earlier failed test, not a second distinct test. |
| Exact46fc API + frontend9c75, reviewed runner augmentation and one independent browser test | **8 PASS /1 intentional SKIP**: existing offline suite7 PASS plus one persisted synthetic timestamp case1 PASS. The real-SEC test is deliberately skipped; no SEC request was made. |
| Exact46fc reachable Git history, redacted gitleaks8.30.1 + unchanged exact-waiver gate | **31 commits; 2 findings, 2 exact historical waivers, 0 open, 0 unused waivers.** Only W-001 and W-002 apply. No new waiver or history rewrite. |

The installed2b wheel SHA256 is `7bb6d0b24df2da7dad07aa732c1d99099078eca404332083be67163ad1ae1e04`. Installed contracts/generated types/embedded documents matched source bytes and lived inside the private virtual environment, with no editable/source fallback. CLI and differential tests used that installed package. The PG runner selects the private copied managed base CPython through `toolchain.sh python`; its embedded-parser run is **not** claimed to use the wheel interpreter. Tool versions were Go1.26.10, CPython3.14.8 and uv0.13.4; setup was offline using already verified owned runtimes/caches.

## Findings and correction history

1. **P2, UTC serialization: resolved at46fc.** The original failure was a wire spelling mismatch, not a wrong instant or a retrospective incident claim. Job/source/event/status times are converted on read; old cooldown detail is normalized only in the served copy. New cooldown detail uses UTC for the same instant. No schema relaxation, migration, source/result rewrite or global timezone change was added.
2. **P2, generator preflight/ref bounds: resolved atdfee.** The original generator constructed output before shared compilation and followed typeless references without a bound. The fix preflights before generation/gofmt/writes and bounds both field and array-item reference walks. Static finding plus passing eight regressions are retained separately.

Earlier draft failures are retained rather than rewritten as always passing: native regex differences/Python recursion; signed repetition counts and cached-depth bypass; compatibility exact-value/input-presence errors; empty-lock acceptance. Fixed parser9, NFA/depth13 per language, six compatibility methods/ten assertions and the separate one-method lock regression are referenced at their recorded hashes. Final Go/Python engine bytes match the successfully reviewed versions. Nested repetition expectations changed explicitly when both engines adopted bounded nonbacktracking Thompson NFAs; small actual matches passed, with no general stress claim. The old long-symbol memo's inferred `INSUFFICIENT_DATA` wording was corrected to actual `NO_TRADE`; the original schema-length mismatch itself was real and fixed.

Two setup diagnostics belonged to reviewer wrappers, not the product: private frontend checkout permissions under umask077 failed a full filesystem-mode assertion, and the history wrapper initially confused archive tar permission bits with Git's executable-bit mode. Source bytes were intact; narrow reviewed resumes corrected those checks without deletion/reclone or canonical changes. A versioned `go run` scanner bootstrap then refused an offline deprecation lookup before scanning. The same cached exact scanner was compiled offline with `-mod=readonly` and run successfully; all failed setup logs remain. Scanner binary SHA256: `86346b8fcb211179690d610e8e30c64f17e1c77593ce5448f1868e17f80d531e`.

The author reported an amend from an unannounced699d candidate before freezing46fc to separate a false-positive-adjacent line. That is attributed to the author/coordinator observation, not an independent execution result. The actual independent scan covers reachable46fc history only; no claim is made that699d is reachable or that it was independently scanned.

## Isolation, source parity and retained resources

Each frozen snapshot retained its tracked byte hashes and full captured filesystem modes after every stage. Across2b→46fc, Git diff plus independent byte comparison identifies exactly eight changed/added paths: generator and its new test, research service/worker and new timezone test, and three documentation files. Existing tracked Git executable modes did not change. The earlier manifest-record comparison included an extra size field and is not used as a source-change verdict; the normalized comparison is `semantic-source-delta.json`.

Final canonical read-only check: HEAD46fc/tree as above, all336 tracked bytes match the tested snapshot, no tracked dirty changes; frontend HEAD9c75 unchanged. Private frontend tracked bytes/modes also remained exact after the run; only the declared untracked test was added. Backend snapshots have no protected configuration. Private history clone has no remote and matched all336 bytes and tracked Git modes.

Disposable PG16.15 used pinned image `sha256:f4610795587492cd4fb887c2dc3de301b813278bb6f616992061141b5f15ac36`, tmpfs data, fresh credentials and verified local Unix Docker endpoint. No normal55432 cluster or retained18532 reviewer resource was used.

- Successful focused-PG run: container `b729f03dde1462d87e2aa561647b66f42110500af64f0ac37b41212ae2e9ef90`, name `tradestocker-t05-test-pg-c7220a672a1b`, loopback51438; exact-ID/label stop, separately confirmed absent.
- Browser run: database `tradestocker_test_f2244f08c031`, container `016bfd980e726e771aed3cb23fd3fe23557cf9f502f15d9b8a14d0389ce571e7`, run label `f2244f08c031`, loopback51495; guarded seed inserted one explicitly synthetic FAILED job/event only. No report, originals or fake SEC links. API process10890 and exact labelled PG stopped; separate inspect confirmed absent. Ports18090/15173 were confirmed released. Test output, binary and browser artifacts remain in the owned review folder.

The coordinator's separate `root-final-normal-preservation.json` at15:38:15UTC records actual read-only preservation: normal PAPER hash `51285359c2d98893a4d47ead4c2fb4da`, research aggregate `1:3c0d1aec0b92becfac3b1148ebaf1b54`, two originals, original PIDs78497/78502, health200, frontend9c75 clean, protected config matching backup/ignored/mode0600 and prior CI PG still stopped. This is explicitly **coordinator evidence**, not a reviewer database query.

## Limits and evidence location

Handlers/consumers do not all invoke these validators in production; the contracts are executable through shared libraries, generation, tools and tests. Relations such as issuer identity, arithmetic, account ownership and whether a reported execution is in the past require runtime gates. The local owner token remains a development stand-in. Automatic HEAD/router404/405 are documented exclusions from OpenAPI operation counts. Free-market data licensing, production identity, brokers and other future gates remain unresolved by this task.

No hosted CI, broad old lifecycle/container suites, new dependency vulnerability scan, stress qualification or live provider flow is claimed here. No fresh SEC, model, Telegram, broker or trading call occurred. No canonical source/state/staging/Git change, push, security-setting change or normal-service restart was performed by this reviewer.

Raw drivers, manifests, clearances, logs and earlier failures remain under external reviewer storage `work/reviewer-research/s02/`: broad baseline `final-2b0149-1s7ddu4i/`, generator correction `generator-dfeeaed-m8yfbk52/`, final UTC/browser/history `utc-46fc10d-sqxizx2q/`. The coordinator may copy this report literally into `docs/evidence/executable-contracts-independent-review.md` after coordinating the author's Git window. Future documentation-only commits require an exact scope/source and reachable-history recheck before final task closeout; unchanged runtime results should be reused.
