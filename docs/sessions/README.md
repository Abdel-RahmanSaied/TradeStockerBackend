# Session Handoffs

Use `../templates/SESSION_HANDOFF.md`. Store one uniquely named handoff per work session. `../state/project_state.json.last_handoff` points to the latest acknowledged handoff.

A session log is historical evidence, not the canonical task-status database. Do not rely on chat history or a single mutable CURRENT_STATUS.md that overwrites the previous session’s facts.

Repository inspection, branch/commit, tests, uncommitted changes, blockers and exact next action are required. Never include secrets or full licensed datasets.
