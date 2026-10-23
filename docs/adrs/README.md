# Architecture Decision Records

These capture the agreed direction and proposed implementation choices. **Accepted scope does not mean implemented.** S00 must reconcile existing ADR numbers and preserve older records.

| ID | Decision | Status | First task |
|---|---|---|---|
| [ADR-001](ADR-001.md) | Modular Go core with three entrypoints | Accepted scope baseline; physical paths require S00 reconciliation | S01-T01 |
| [ADR-002](ADR-002.md) | Go authority and Python research | Accepted scope baseline | S02-T01 |
| [ADR-003](ADR-003.md) | PostgreSQL plus versioned analytical object storage | Accepted design baseline | S02-T02 |
| [ADR-004](ADR-004.md) | JetStream plus outbox/inbox | Accepted design baseline | S02-T04 |
| [ADR-005](ADR-005.md) | Human-only execution as an architectural prohibition | User-approved invariant | S01-T06 |
| [ADR-006](ADR-006.md) | One deterministic policy implementation for live and replay | Accepted design baseline | S05-T01 |
| [ADR-007](ADR-007.md) | Point-in-time evidence and immutable corrections | Accepted scope baseline | S06-T01 |
| [ADR-008](ADR-008.md) | Conditional hierarchical advisory roles | Accepted scope baseline | S08-T01 |
| [ADR-009](ADR-009.md) | Goals cannot expand risk authority | User-approved invariant | S12-T01 |
| [ADR-010](ADR-010.md) | Independent four-lane activation | Accepted scope baseline | S15-T01 |
| [ADR-011](ADR-011.md) | Independent trading core with optional OpenClaw interface | User-approved decision; reconcile any existing ADR-011 at S00 | S16-T01 |
| [ADR-012](ADR-012.md) | Source-of-truth ownership and evidence-backed sessions | Accepted planning convention | S00-T06 |
| [ADR-013](ADR-013.md) | AWS deployment baseline with explicit cost/HA approval | Proposed implementation detail; resolve D-06 | S14-T01 |
| [ADR-014](ADR-014.md) | Separate recommendation, delivery, response and fill states | Accepted design baseline | S05-T03 |
| [ADR-015](ADR-015.md) | Safe implementation discovery and no silent overwrite | Accepted planning convention | S00-T01 |
| [ADR-016](ADR-016.md) | Version-control bootstrap and repository boundary | Proposed; owner decision pending | S00-T01 |

Local prototype proposal: [ADR-017](ADR-017.md). This does not change production architecture approvals.

SEC research design proposal: [ADR-018](ADR-018.md). Local prototype for S07-T07; not a production approval.
