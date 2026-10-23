# Decision options — D-05 identity, D-06 deployment and budget, D-07 workload and targets

| Field | Value |
|---|---|
| Prepared by | `claude-code-agent` (session `2026-10-23-development-8da61cee`), 2026-10-23 |
| Feeds | S00-T05 (BLOCKED on these decisions), S01-T04 (identity), S14-T01 (production platform) |
| Status | **Options for review. No decision is recorded here.** D-05, D-06 and D-07 stay OPEN. |
| Decision authority | D-05: Tech Lead + Security. D-06: Owner + Platform. D-07: Tech Lead + Owner ([open decisions](../OPEN_DECISIONS.md)) |

Nothing was created, bought, configured or implemented for this document: no cloud account, user pool, server, domain or key. Prices are dated list prices from official AWS sources, not quotes. Where this document gives the author's suggestion, it says so; a suggestion is not a choice.

## 0. What is already measured (local only)

These numbers come from the [local load profile](load-and-cost-profile.md) (2026-10-23, one Apple-silicon laptop, synthetic data). They show how the pieces behave on one machine. **They are not production capacity or targets.**

| Measure | Local result |
|---|---|
| JetStream publish, one synchronous publisher | 5,582 messages/s; p95 0.234 ms |
| Consumer with one PostgreSQL transaction per message | 1,159 messages/s |
| One real SEC research job (Apple) | about 2.7 s end to end, limited on purpose to 1 SEC request per second |
| Paper prototype reads after about 400 evaluations | 39/s; p50 201 ms (the whole-document prototype does not scale; ADR-017) |

Still unknown, and needed before any target: symbols per market, feed event rate, research jobs per day, retention periods and the model budget (D-01, D-02, D-07).

## 1. D-05 — Identity, sessions and access

### What the plan already requires

- S01-T04: unauthenticated access to portfolio and research administration is denied; issuer, audience, expiry, CSRF or token handling and service-to-service authorization are tested; separate database roles per service; credentials never reach LLM tools or logs; roles **owner**, **reviewer** and **read-only**; every financial record is account-scoped.
- S00-T05: no custom token cryptography.
- Go platform-core stays the authorization authority. An identity provider only proves who the user is.

[NIST SP 800-63B-4](https://pages.nist.gov/800-63-4/sp800-63b.html) (page dated 2025-08-26) is a useful yardstick:

- At AAL2, verifiers "SHALL offer at least one phishing-resistant authentication option".
- At AAL2 the overall reauthentication timeout "SHOULD be no more than 24 hours", and the inactivity timeout "SHOULD be no more than 1 hour".
- AAL3 needs a phishing-resistant authenticator with a non-exportable key.

### Threats the choice must handle

Phishing of a password; a stolen session cookie or token; CSRF on state-changing calls (paper fills, entry pause); a token accepted for the wrong audience; trusting an unsigned header added by a proxy; a request that names another account ID; abuse of account recovery; service credentials leaking into logs or prompts; the local development token reaching production.

### Options

| | A. Managed sign-in: Amazon Cognito | B. Sign-in at the load balancer (ALB) | C. Self-hosted passkeys in platform-core | D. Private network plus today's development token |
|---|---|---|---|---|
| How it works | A Cognito user pool signs the user in. The Go API checks every access token (signature, issuer, audience or client, token type, expiry) with a maintained library, and maps Cognito groups to owner, reviewer or read-only. | The ALB runs the sign-in (`authenticate-cognito` or `authenticate-oidc`, HTTPS listeners only) and forwards the user's claims in a signed `x-amzn-oidc-data` header. | platform-core registers and checks passkeys (WebAuthn) with a maintained library and keeps server-side sessions in PostgreSQL. | The dashboard is reachable only over a private network; the API keeps the single owner token. |
| Phishing-resistant | Yes: the Essentials plan (the default for new user pools) includes passkey sign-in with FIDO2 authenticators. MFA with authenticator apps is in every plan. | Depends on the identity provider behind the ALB (Cognito Essentials offers passkeys). | Yes: passkeys are phishing-resistant by design. | No. |
| Code the project owns | Token checking and role mapping. Library example: [coreos/go-oidc](https://github.com/coreos/go-oidc) v3.21.0 (Apache-2.0, released 2026-09-01), to be pinned and reviewed in S01-T04. | Checking the ES256 signature of `x-amzn-oidc-data` with the regional public-key endpoint, and that its `signer` field is the expected ALB ARN. The two other `x-amzn-oidc-*` headers are unsigned and must never be trusted. | Passkey registration, sign-in, session revocation and account recovery. Library example: [go-webauthn/webauthn](https://github.com/go-webauthn/webauthn) v0.18.2 (BSD-3-Clause, released 2026-09-19). Session IDs are random values; cookies are HttpOnly, Secure and SameSite. No custom signing or encryption. | None new. |
| Must also handle | Where the browser keeps tokens; CSRF if cookies are used; logout and token revocation. | The ALB session lasts 7 days by default and must be shortened; targets must accept traffic only from the ALB's security group; logout must expire the ALB cookies and redirect to the provider's logout; a login must finish within 15 minutes. | Recovery (for example one-time recovery codes) and a second registered passkey; CSRF tokens on state-changing calls. | Not enough for S01-T04: no per-user identity, roles or expiry. |
| Dated cost | Lite and Essentials share an ongoing free tier of 10,000 monthly active users per account; above it, Essentials costs $0.015 per monthly active user in us-east-1. | The ALB itself: $16.43–$20.60 per month for the hourly part in the three regions priced in section 2, plus capacity units. | No vendor cost. | No cost. |
| Depends on | An AWS account (D-06), though Cognito can serve a non-AWS deployment too. | D-06 option 3 (an ALB). | Nothing external; works in every D-06 option, including local. | — |
| Main risk | Vendor dependency; token handling in the browser. | Identity tied to the AWS topology; local and production sign-in differ. | More security-sensitive code to own and review. | Not a production identity. |

Sources: [Cognito pricing](https://aws.amazon.com/cognito/pricing/), [Cognito feature plans](https://docs.aws.amazon.com/cognito/latest/developerguide/cognito-sign-in-feature-plans.html), [ALB user authentication](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html), the AWS price list (section 2), and the GitHub release pages of both libraries (read 2026-10-23).

**Service-to-service access (every option):** a separate PostgreSQL role per service (core, ingestion, research), with no shared superuser; separate NATS credentials per service (method chosen in S01-T04); cloud permissions from task roles rather than long-lived keys (as CI already plans for AWS: [SRC-09](../SOURCES.md)). The Python research worker never receives user tokens or broker credentials.

### Test plan (for whichever option is chosen; maps to S01-T04)

1. Every protected route answers 401 without a session or token. A test reads the route list, so a new route cannot be left open.
2. A token or session that is expired, from the wrong issuer, for another audience or client, or with a broken signature is refused.
3. A read-only user cannot call any state-changing route. A reviewer cannot record fills or change risk settings.
4. Passing another account's ID returns 404 or 403 and reveals nothing.
5. State-changing calls without a valid CSRF token fail (if cookies are used).
6. Logout and revocation end the session on the server side.
7. Option B only: a request that reaches the target without the ALB's signed header, or with a header signed by another load balancer, is refused.
8. Logs, traces and research prompts contain no token, cookie or credential (scanned in tests).
9. The local development token is refused by any non-local configuration.

**Author's suggestion (not a decision):** for a single-owner private pilot, option C keeps identity inside the Go authority and works the same in every D-06 option. Option A is a good fit if D-06 chooses AWS and a managed service is preferred. Option B makes sense only with the full AWS topology. Option D is not a production option; it can only add a private-network layer in front of A, B or C. Security reviews and chooses.

## 2. D-06 — Deployment, availability and budget

ADR-013 proposes ECS app workers, RDS and S3 with durable NATS, and says a single VM is not high availability. The options below include that baseline and two smaller steps for a private pilot.

| | 1. Local only | 2. One small cloud server | 3. AWS baseline (ADR-013) |
|---|---|---|---|
| Shape | The owner's own computer runs everything, as today. | One Amazon Lightsail server with Docker Compose: PostgreSQL, one NATS server, the app. Daily backups to S3. | ECS on Fargate (ARM) for the services, RDS for PostgreSQL, an ALB, S3, and a three-server NATS cluster. |
| Availability | Only while the computer is on and online. | One server in one zone: no failover. | Survives the loss of one zone if RDS is Multi-AZ and NATS uses three replicas. |
| Recovery | Backups to an external disk or S3. | Restore the server and the database from backup. | RDS point-in-time restore; NATS replicas. |
| Telegram | Long polling (`getUpdates`) needs no inbound port. Webhooks and long polling are "two mutually exclusive ways" to receive updates. | Long polling, or a webhook on ports 443, 80, 88 or 8443 with the `X-Telegram-Bot-Api-Secret-Token` header. | Same as option 2. |
| Fits | PAPER and SHADOW; swing lanes at most. | PAPER, SHADOW, swing pilot. | Any lane, including intraday, after its own qualification. |

### Dated cost estimates (USD per month)

On-demand list prices from the official [AWS price list files](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/index.json), fetched 2026-10-23 (publication dates 2026-09-11 to 2026-10-20 depending on the service), times 730 hours. The three regions are examples only; the region is an open question.

Assumptions:

- Option 2: one Lightsail Linux bundle with a public IPv4 address, plus 20 GB in S3 Standard.
- Option 3:
  - Fargate ARM, always on: 1.75 vCPU and 3.5 GB in total (core 0.5 vCPU / 1 GB, ingestion 0.25 / 0.5, research 1 / 2).
  - RDS PostgreSQL `db.t4g.small` with 20 GB of gp3 storage.
  - One ALB averaging one capacity unit.
  - Five public IPv4 addresses (two for the ALB, three for tasks, so that no NAT gateway is needed).
  - 20 GB in S3.

| Region | Option 2, 4 GB server | Option 2, 8 GB server | Option 3, single-zone database | Option 3, Multi-AZ database |
|---|---|---|---|---|
| us-east-1 (N. Virginia) | 24.00 | 43.62 | 117.10 | 143.49 |
| eu-central-1 (Frankfurt) | 24.03 | 43.65 | 132.08 | 161.83 |
| me-central-1 (UAE) | not offered in the price list (no Lightsail file for this region) | — | 142.05 | 172.57 |

Option 1 has no hosting cost; 20 GB of S3 backups would add about $0.46–$0.50 per month.

Option 3 in us-east-1, item by item:

| Item | USD per month |
|---|---|
| Fargate | 50.46 |
| RDS, single zone (instance 23.36 + storage 2.30) | 25.66 |
| RDS, Multi-AZ (instance 47.45 + storage 4.60), instead of the line above | 52.05 |
| ALB | 22.27 |
| Five public IPv4 addresses | 18.25 |
| S3 | 0.46 |

Unit prices used:

| Unit | us-east-1 | eu-central-1 | me-central-1 |
|---|---|---|---|
| Fargate ARM vCPU-hour / GB-hour | 0.03238 / 0.00356 | 0.03725 / 0.00409 | 0.04208 / 0.00464 |
| RDS PostgreSQL `db.t4g.small`, single zone / Multi-AZ, per hour | 0.032 / 0.065 | 0.037 / 0.074 | 0.038 / 0.076 |
| RDS gp3 storage per GB-month, single zone / Multi-AZ | 0.115 / 0.23 | 0.137 / 0.274 | 0.14 / 0.279 |
| ALB per hour / per capacity-unit-hour | 0.0225 / 0.008 | 0.027 / 0.008 | 0.028224 / 0.00896 |
| Public IPv4 address per hour | 0.005 | 0.005 | 0.005 |
| S3 Standard per GB-month (first 50 TB) | 0.023 | 0.0245 | 0.025 |
| Lightsail 4 GB / 8 GB bundle with IPv4, per hour | 0.03225 / 0.05913 | 0.03225 / 0.05913 | — |

**Not included:**

- NATS servers for option 3. EC2 instances and EBS volumes were not priced: the EC2 price file is 481 MB and was not downloaded.
- A NAT gateway, data transfer out, CloudWatch logs and metrics, Secrets Manager, KMS, Route 53 and Lightsail snapshots.
- Backup storage beyond the free allocation, support plans and taxes.
- Credits, savings plans and the separate AWS Free Tier.
- Market data (D-01, D-02) and model usage (D-07). These are not hosting costs.

The real option-3 total is higher than the table.

### Recovery facts that limit RTO and RPO

- RDS uploads transaction logs to S3 every five minutes; a restore can go to any point within the backup retention period, which can be 0 to 35 days (0 turns automated backups off). Sources: [point-in-time restore](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_PIT.html), [backup retention](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_WorkingWithAutomatedBackups.BackupRetention.html).
- A Multi-AZ RDS instance keeps a synchronous standby in another zone; the standby does not serve reads ([Multi-AZ](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZSingleStandby.html)). The page read gives no failover time, so none is assumed here.
- NATS: a stream has one copy (R1) unless asked for more; losing that server loses the stream. Three copies (R3) are "the production floor" and survive one server loss if the servers fail independently. A cluster needs at least three servers, in odd numbers ([JetStream clustering](https://docs.nats.io/running-a-nats-service/configuration/clustering/jetstream_clustering)).

**Author's suggestion (not a decision):** run PAPER and SHADOW for the private pilot on option 1 or 2. Move to option 3, with a Multi-AZ database and three NATS servers, before any intraday lane where a missed invalidation matters. The owner decides the budget and the region.

## 3. D-07 — Workload profile and targets

A target counts only after the owner preregisters it, with how it will be measured, before the measurement (S14). The example values below are there to react to. They are **not** recommendations backed by measurement; the local numbers in section 0 do not support any production value.

| Metric | How it will be measured | Example: swing pilot (option 1 or 2) | Example: intraday-capable (option 3) |
|---|---|---|---|
| Price snapshot age when a recommendation is finalized | provider timestamp versus decision time, per recommendation | at most 15 minutes | at most 5 seconds |
| Delay from decision to Telegram send | outbox time versus Telegram API answer | at most 5 minutes | at most 30 seconds |
| Delay to send an invalidation | trigger event versus send | at most 15 minutes | at most 30 seconds |
| Recovery time (RTO) | timed restore drill | 24 hours | 15 minutes during market hours |
| Data loss allowed (RPO), ledger | restore drill: last committed entry versus restored state | 24 hours; manual entries re-imported from statements | 5 minutes (the RDS log upload interval) |
| Hosting cost | AWS bill | owner's monthly cap | owner's monthly cap |
| Model cost | metered calls | owner's daily cap, with a hard stop | same |

Workload inputs the owner or a vendor must supply first:

- symbols per market;
- for intraday, the feed rate (D-01, D-02);
- research jobs per day;
- how many people sign in (owner only, or owner and reviewer);
- retention periods for sources, events and audit records (D-08 may affect these).

## 4. Questions for the owner

Short answers are enough. Nothing here asks for keys, passwords or account numbers.

1. **Hosting (D-06):** local only, one small cloud server, or the AWS baseline?
2. **Budget (D-06, D-07):** the highest monthly amount you accept for hosting, and a daily cap for model usage.
3. **Region (D-06):** a preferred AWS region, if any. Where you live is not guessed from timezone or signups.
4. **Downtime (D-07):** during market hours, is a few hours of downtime acceptable for the pilot (swing), or must it recover within minutes (intraday)?
5. **People (D-05):** only you, or you and a reviewer? Any read-only viewer?
6. **Sign-in (D-05):** do you prefer a managed service (option A) or sign-in kept inside the platform (option C)? Either way, you would sign in with a passkey on your phone or computer.

## Limits

- The prices are list prices on one date. They change, and the estimate leaves out items listed above.
- The identity options were read in vendor and standards documentation; none was built or tested here.
- The example targets are prompts for a decision, not commitments.
