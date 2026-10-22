# Instructions for this docs tree

Read SOURCE_OF_TRUTH.md and AGENT_PLAYBOOK.md before changing plans or status. Task definitions are canonical in state/task_catalog.json; task status is canonical in state/project_state.json. Do not hand-edit generated sprint documents or duplicate live status in prose.

Run `python3 docs/tools/planctl.py validate` and `render --check` from the repository root. Evidence must be real and complete; all initial TODO states are intentional. Never infer implementation or deployment from this planning pack.

Preserve existing parent AGENTS instructions and user changes. When sources conflict, record the discrepancy; do not silently override safety boundaries. No broker execution or target-driven risk escalation may be added.
