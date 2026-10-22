# Security, CI/CD and Operations

## Threat boundaries

Untrusted sources include filings, websites, social/news text, uploaded statements, Telegram payloads and LLM outputs. Validate them as data. Never accept embedded instructions to run shell commands, disclose credentials, change risk or call broker actions.

Go core is the authority for user/account authorization. Research gets scoped inputs and writes only its own result schema. No shared DB superuser. Account identifiers from requests or events are not authorization on their own.

## Baseline controls from S01, not a final cleanup sprint

- Authentication and scoped authorization, audit of privileged changes, secure transport and secret management.
- Fetch allowlists, SSRF/private-network protections, bounded file parsing, sandboxing of risky extractors and archive-bomb limits.
- Evidence/content storage encryption, protected access links, retention and provider-use restrictions.
- Non-root containers, pinned dependencies/images, build provenance, SBOM, secret/dependency scanning and reviewed expiring exceptions.
- No production secrets in local fixtures, reports, prompts, logs or screenshots.
- Telegram paired identities, server-side short-lived action references and webhook secret validation [SRC-05](SOURCES.md).

## CI lanes

**PR:** docs validation; generated schema/type drift; Go format/vet/test/race; Python lint/type/unit; OpenAPI/event validation; integration tests with PostgreSQL/NATS; secret/dependency scans. Untrusted PRs receive no cloud or provider secrets.

**Protected integration:** golden data/policy parity, migration upgrade tests, ledger races, prompt-injection and research eval fixtures, multi-service end-to-end scenarios. Real provider/model tests are explicitly permissioned and budgeted.

**Release:** immutable artifact digests and provenance; full manifest; protected environment approval; Terraform plan review; expand/contract migrations; read-only deployment smoke tests; explicit rollback/recovery evidence. GitHub OIDC scopes AWS trust to the correct repo/ref/environment rather than long-lived keys [SRC-09](SOURCES.md).

**Independent activation:** a deployment starts with lanes disabled. A separate recorded owner decision enables a previously qualified lane. CI success never serves as investment or trading authority.

## Observability

Track queue/backlog age, source timestamps/freshness, received-versus-provider delay, data-quality reasons, subscription ownership, invalidated reports, LLM/model cost, job retry/dead-letter counts, recommendation rejection reasons, ledger reconciliation age, unknown message delivery and actual human response time where observable.

Keep high-cardinality identifiers in logs/traces, not metric labels. Trace event→assessment→policy→recommendation→delivery→manual execution through correlation IDs. No sensitive financial/document payload is required in telemetry.

## Recovery model

PostgreSQL restore is tested against the ledger and outbox/inbox, not merely a successful backup status. Preserve original evidence hashes and manifests. After recovery, reconcile account state, validate feed freshness and suppress historical message replay before allowing new advice.

Durable NATS storage, replication and topology must satisfy the chosen failure model. A single-host development setup is not high availability. Production HA choice and cost are D-06; actual RTO/RPO are D-07, then measured under S14.

## Runbooks to implement

Provider outage; clock/data corruption; bad corporate action; incorrect financial extraction; model outage/budget exhaustion; Telegram token compromise/UNKNOWN send; ledger discrepancy; entry pause; DB restore; NATS consumer recovery; revoked provider license; lost human availability; unexpected overnight intraday holding.

Every runbook names owner, detection, immediate containment, safe degraded behavior, evidence preservation, recovery checks and entry-resume authorization. This document lists runbooks to build; it does not claim they exist yet.
