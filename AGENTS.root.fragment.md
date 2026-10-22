<!-- TRADESTOCKER_PLAN_START -->
## TradeStocker planning and session continuity

Before work, read `docs/SOURCE_OF_TRUTH.md`, `docs/AGENT_PLAYBOOK.md`, `docs/state/project_state.json` and its latest handoff. Inspect Git status/branch/HEAD and existing instructions; do not assume an empty repository.

Run `python3 docs/tools/planctl.py validate`, `status` and `ready`. Claim a task with owner/session and current revision. Implement its acceptance criteria; preserve unrelated work. At the end, record evidence and a session handoff, update the single execution-state file and validate again.

Fixed scope: Go authority + Python AI/Quant; EGX/US Swing and Intraday; Telegram; human-only execution. No broker order API/credentials, no unapproved risk changes, no target-driven risk escalation. OpenClaw is optional read-only and last.

This docs pack is a development baseline, not evidence of implemented or deployed product code. First local task is S00-T01 unless verified state already records it complete. Never mark DONE without actual verification/review. Existing higher-priority instructions remain in force; resolve conflicts explicitly.
<!-- TRADESTOCKER_PLAN_END -->
