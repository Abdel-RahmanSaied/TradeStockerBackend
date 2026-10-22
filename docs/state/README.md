# Canonical planning state

`task_catalog.json` owns task specifications: IDs, sprint membership, dependencies, deliverables, acceptance criteria, validations and requirement links. The generator renders DEVELOPMENT_PLAN.md and sprints/*.md from it.

`project_state.json` owns execution status. It includes optimistic revision, observed repository identity, task owners/sessions/evidence/reviews, lane approval records, external decision statuses and history. Every write increments revision and appends a history event. Do not add duplicate manual status fields to sprint documents.

Initial values are intentionally conservative: repository NOT_INSPECTED, all tasks TODO, all lanes DISABLED, decisions OPEN. These are not statements about the unknown current user repository; they mean this package has no verified evidence yet. S00 reconciles real state.

Safe transitions use ../tools/planctl.py. Direct reviewed edits needed for new tasks, repository observations, external decisions or reassignment must increment revision, append history and pass validation. The CLI is not a distributed lock or an authenticated approval system. Lane records are documentary and do not control deployment/runtime.
