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
| How it works | A Cognito user pool signs the user in. The Go API accepts only access tokens, under an explicit access-token policy: signature from the user pool's keys (Cognito signs access and ID tokens with different keys and says to verify them independently), issuer, `token_use` = `access`, `client_id` = the expected app client, required `scope`, `aud` only if a resource binding is used (an access token has `aud` only then), and expiry. Go maps Cognito groups to owner, reviewer or read-only. | The ALB runs the sign-in (`authenticate-cognito` or `authenticate-oidc`, HTTPS listeners only) and forwards the user's claims in a signed `x-amzn-oidc-data` header. | platform-core registers and checks passkeys (WebAuthn) with a maintained library and keeps server-side sessions in PostgreSQL. | The dashboard is reachable only over a private network; the API keeps the single owner token. |
| Phishing-resistant | Yes: the Essentials plan (the default for new user pools) includes passkey sign-in with FIDO2 authenticators. MFA with authenticator apps is in every plan. | Depends on the identity provider behind the ALB (Cognito Essentials offers passkeys). | Yes: passkeys are phishing-resistant by design. | No. |
| Code the project owns | The access-token policy above, as a small adapter around a maintained JWT/JWKS library, plus role mapping and negative tests. [coreos/go-oidc](https://github.com/coreos/go-oidc) v3.21.0 (Apache-2.0, released 2026-09-01) is an OIDC client whose README shows **ID-token** verification with a client ID; it is not a drop-in access-token verifier. The library is chosen, pinned and reviewed in S01-T04. | Checking the ES256 signature of `x-amzn-oidc-data` with the regional public-key endpoint, and that its `signer` field is the expected ALB ARN. The two other `x-amzn-oidc-*` headers are unsigned and must never be trusted. | Passkey registration, sign-in, session revocation and account recovery. Library example: [go-webauthn/webauthn](https://github.com/go-webauthn/webauthn) v0.18.2 (BSD-3-Clause, released 2026-09-19). Session IDs are random values; cookies are HttpOnly, Secure and SameSite. No custom signing or encryption. | None new. |
| Must also handle | Where the browser keeps tokens; CSRF if cookies are used; logout and token revocation. | The ALB session lasts 7 days by default and must be shortened; targets must accept traffic only from the ALB's security group; logout must expire the ALB cookies and redirect to the provider's logout; a login must finish within 15 minutes. | Recovery (for example one-time recovery codes) and a second registered passkey; CSRF tokens on state-changing calls. | Not enough for S01-T04: no per-user identity, roles or expiry. |
| Dated cost | Users who sign in directly or through a social provider, on Lite or Essentials: an ongoing free tier of 10,000 monthly active users per account or AWS organization (Plus has no free tier). Users federated through SAML 2.0 or OIDC: 50 free. Machine-to-machine token requests: no free tier, charged per successful token response. SMS (Amazon SNS) and email (Amazon SES) are charged separately. Above the free tier, Essentials costs $0.015 per monthly active user in each of the three regions priced (appendix). | The ALB itself: $16.43–$20.60 per month for the hourly part in the three regions priced in section 2, plus capacity units. | No vendor cost. | No cost. |
| Depends on | An AWS account (D-06), though Cognito can serve a non-AWS deployment too. | D-06 option 3 (an ALB). | Nothing external; works in every D-06 option, including local. | — |
| Main risk | Vendor dependency; token handling in the browser. | Identity tied to the AWS topology; local and production sign-in differ. | More security-sensitive code to own and review. | Not a production identity. |

Sources: [Cognito pricing](https://aws.amazon.com/cognito/pricing/), [Cognito feature plans](https://docs.aws.amazon.com/cognito/latest/developerguide/cognito-sign-in-feature-plans.html), [Cognito access token](https://docs.aws.amazon.com/cognito/latest/developerguide/amazon-cognito-user-pools-using-the-access-token.html), [go-oidc v3.21.0 README](https://raw.githubusercontent.com/coreos/go-oidc/v3.21.0/README.md), [ALB user authentication](https://docs.aws.amazon.com/elasticloadbalancing/latest/application/listener-authenticate-users.html), the AWS price list (section 2), and the GitHub release pages of both libraries (read 2026-10-23).

**Service-to-service access (every option):** a separate PostgreSQL role per service (core, ingestion, research), with no shared superuser; separate NATS credentials per service (method chosen in S01-T04); cloud permissions from task roles rather than long-lived keys (as CI already plans for AWS: [SRC-09](../SOURCES.md)). The Python research worker never receives user tokens or broker credentials.

### Test plan (for whichever option is chosen; maps to S01-T04)

1. Every protected route answers 401 without a session or token. A test reads the route list, so a new route cannot be left open.
2. A token or session that is expired, from the wrong issuer, for another audience or client, or with a broken signature is refused. Option A also: an ID token presented as an access token (`token_use` = `id`), a token for another `client_id`, a token missing the required `scope`, and, if a resource binding is used, a token for another `aud` are each refused.
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
| Availability | Only while the computer is on and online. | One server in one zone: no failover. | Zone-loss survival is a design requirement, not a property of these parts. It needs at least: every service running in more than one zone with rescheduling (the cost table below has one task per service); a Multi-AZ database; a three-server NATS cluster spread over zones, on durable storage, with replicated streams and quorum; clients that reconnect; and a failure drill that qualifies it. None of this is designed or tested yet. |
| Recovery | Backups to an external disk or S3. | Restore the server and the database from backup. | RDS point-in-time restore and NATS replicas, once designed and proven by timed drills. |
| Telegram | Long polling (`getUpdates`) needs no inbound port. Webhooks and long polling are "two mutually exclusive ways" to receive updates. | Long polling, or a webhook on ports 443, 80, 88 or 8443 with the `X-Telegram-Bot-Api-Secret-Token` header. | Same as option 2. |
| Fits | PAPER and SHADOW; swing lanes at most. | PAPER, SHADOW, swing pilot. | Any lane, including intraday, after its own qualification. |

### Dated cost estimates (USD per month): component subtotals

On-demand list prices from the official [AWS price list files](https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/index.json), fetched 2026-10-23 (publication dates 2026-09-11 to 2026-10-20 depending on the service), times 730 hours: a representative month, not a calendar bill. Every exact rate code, file version and file hash is in the appendix. The three regions are examples only; the region is an open question.

These are **subtotals of the listed parts only**. They are not the price of a zone-resilient topology: option 3 here has one task per service and no NATS servers, so it is not the topology the availability row above requires.

Assumptions:

- Option 2: one Lightsail Linux bundle with a public IPv4 address, plus 20 GB in S3 Standard.
- Option 3:
  - Fargate ARM, always on: 1.75 vCPU and 3.5 GB in total (core 0.5 vCPU / 1 GB, ingestion 0.25 / 0.5, research 1 / 2).
  - RDS PostgreSQL `db.t4g.small` with 20 GB of gp3 storage.
  - One ALB averaging one capacity unit.
  - Five public IPv4 addresses (two for the ALB, three for tasks, so that no NAT gateway is needed).
  - 20 GB in S3.

Option 2 (Lightsail). The [Lightsail pricing page](https://aws.amazon.com/lightsail/pricing/) advertises the Linux bundles with a public IPv4 address at **$24 per month** (4 GB) and **$44 per month** (8 GB). That page does not say how the hourly rates in the price list relate to the monthly price (for example in a 31-day month), so the 730-hour figures are shown only as an illustration.

| Region | 4 GB: advertised $24 + 20 GB S3 | 8 GB: advertised $44 + 20 GB S3 | 730-hour illustration, 4 GB / 8 GB (+ S3) |
|---|---|---|---|
| us-east-1 (N. Virginia) | 24.46 | 44.46 | 24.00 / 43.62 |
| eu-central-1 (Frankfurt) | 24.49 | 44.49 | 24.03 / 43.65 |
| me-central-1 (UAE) | no Lightsail price file for this region in the price list (HTTP 404 on 2026-10-23); availability there was not checked any other way | — | — |

Option 3 (AWS baseline), component subtotal:

| Region | Single-zone database | Multi-AZ database |
|---|---|---|
| us-east-1 (N. Virginia) | 117.10 | 143.49 |
| eu-central-1 (Frankfurt) | 132.08 | 161.83 |
| me-central-1 (UAE) | 142.05 | 172.57 |

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

The real total for option 3, and especially for a zone-resilient version of it, is higher than these subtotals.

### Recovery facts that limit RTO and RPO

- RDS uploads transaction logs to S3 every five minutes, and a restore can go to any point within the backup retention period, which can be 0 to 35 days (0 turns automated backups off). AWS points to the `LatestRestorableTime` value for the latest point that can actually be restored. The upload cadence is **not** a guaranteed bound on data loss, and it says nothing about the ledger after a restore. Any RPO needs the observed `LatestRestorableTime` lag plus timed restore and ledger-reconciliation drills before it is chosen. Sources: [point-in-time restore](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_PIT.html), [backup retention](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/USER_WorkingWithAutomatedBackups.BackupRetention.html).
- A Multi-AZ RDS instance keeps a synchronous standby in another zone; the standby does not serve reads ([Multi-AZ](https://docs.aws.amazon.com/AmazonRDS/latest/UserGuide/Concepts.MultiAZSingleStandby.html)). The page read gives no failover time, so none is assumed here.
- NATS: a stream has one copy (R1) unless asked for more; losing that server loses the stream. Three copies (R3) are "the production floor" and survive one server loss if the servers fail independently. A cluster needs at least three servers, in odd numbers ([JetStream clustering](https://docs.nats.io/running-a-nats-service/configuration/clustering/jetstream_clustering)).

**Author's suggestion (not a decision):** run PAPER and SHADOW for the private pilot on option 1 or 2. Before any intraday lane where a missed invalidation matters, design, price and qualify the zone-resilient form of option 3 (the requirements in the availability row). The owner decides the budget and the region.

## 3. D-07 — Workload profile and targets

A target counts only after the owner preregisters it, with how it will be measured, before the measurement (S14). The example values below are there to react to. They are **not** recommendations backed by measurement; the local numbers in section 0 do not support any production value.

| Metric | How it will be measured | Example: swing pilot (option 1 or 2) | Example: intraday-capable (option 3) |
|---|---|---|---|
| Price snapshot age when a recommendation is finalized | provider timestamp versus decision time, per recommendation | at most 15 minutes | at most 5 seconds |
| Delay from decision to Telegram send | outbox time versus Telegram API answer | at most 5 minutes | at most 30 seconds |
| Delay to send an invalidation | trigger event versus send | at most 15 minutes | at most 30 seconds |
| Recovery time (RTO) | timed restore drill | 24 hours | 15 minutes during market hours |
| Data loss allowed (RPO), ledger | observed `LatestRestorableTime` lag, then a timed restore drill: last committed entry versus restored state, followed by ledger reconciliation | 24 hours; manual entries re-imported from statements | 5 minutes, as an unvalidated candidate only: RDS's five-minute log upload is not a guaranteed bound, so it must be proven by the observed lag and the drills |
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

## Appendix: pricing sources

Every price in section 2 comes from one of these public AWS price list files. They are not committed. Each can be fetched again at `https://pricing.us-east-1.amazonaws.com/offers/v1.0/aws/<Service>/<Version>/<region>/index.json`; the version-specific path names the exact version read, while `current` can later point to a newer version. Spot check on 2026-10-24: three versioned URLs answered HTTP 200, and the ELB me-central-1 file had the same SHA-256 as the copy below. The author fetched the `current` files on 2026-10-23. The independent reviewer selected the same 37 price dimensions from the same file hashes (`selected-price-dimensions.json`, SHA-256 `26e7a51b…`), and the author's own selection matched every one.

| Service, region | Version | Published (UTC) | SHA-256 |
|---|---|---|---|
| AmazonECS, us-east-1 | 20261019224317 | 2026-10-19T22:43:17Z | `b30483c1fc2e9446987bae34cbbd593ece4995f0adb1cb6e25e2d4471f14e612` |
| AmazonECS, eu-central-1 | 20261019224317 | 2026-10-19T22:43:17Z | `396790c335e837864fd22792fc3abf6243072ad4f92554819b42eae5d07f706b` |
| AmazonECS, me-central-1 | 20261019224317 | 2026-10-19T22:43:17Z | `63063ee40570efc87ed1b25edce1d4561da7bdbb52d5c694af46801a417fb8dc` |
| AmazonRDS, us-east-1 | 20261020003232 | 2026-10-20T00:32:32Z | `21ad3109d81fe992945d29d8e9f27c030879bbef112c98685bdd57e27b7d1131` |
| AmazonRDS, eu-central-1 | 20261020003232 | 2026-10-20T00:32:32Z | `d8c1a7420dd7f807cf3049e014505b5ceaa36d3bf3545160653afae068ea79f3` |
| AmazonRDS, me-central-1 | 20261020003232 | 2026-10-20T00:32:32Z | `9b53cffbd9f48da6a9db537776d968672393deef556fb9263dd4ac4f62bb7c69` |
| AWSELB, us-east-1 | 20260911124544 | 2026-09-11T12:45:44Z | `c34f95e97c33d8b32b25eecb2e2008ba651a11ba3bc69794200f5749dcb9d463` |
| AWSELB, eu-central-1 | 20260911124544 | 2026-09-11T12:45:44Z | `097a6420aa923b9e495a7c4a2eb994f0980b5c685ffe8c4a984ba32ae5b86291` |
| AWSELB, me-central-1 | 20260911124544 | 2026-09-11T12:45:44Z | `06992ce59dd5b290b02c57c058751f866c9c7208be9a1310ca449b179ff5218b` |
| AmazonVPC, us-east-1 | 20261016224321 | 2026-10-16T22:43:21Z | `85367cb441bc050ea66796736a4605b4a937f9b2a8639a7103270c7eac941a1c` |
| AmazonVPC, eu-central-1 | 20261016224321 | 2026-10-16T22:43:21Z | `727c6816e9bda92f53884dc3630b3019b17d86ef49a5591ce97ec27ab49fcb3a` |
| AmazonVPC, me-central-1 | 20261016224321 | 2026-10-16T22:43:21Z | `394e3a3f71325623070a7dffba9f7c97a1a5065b031be87944d80b8e4df9eb82` |
| AmazonS3, us-east-1 | 20261015173540 | 2026-10-15T17:35:40Z | `a298361f29bd9bc926d810d1eb093720827275689c1e2db5a7af90890b1dc67c` |
| AmazonS3, eu-central-1 | 20261015173540 | 2026-10-15T17:35:40Z | `77eb8a14e486d1431f3b1b5c25b81670d3c533c547d34ef10b2bd65f0ee6b471` |
| AmazonS3, me-central-1 | 20261015173540 | 2026-10-15T17:35:40Z | `250d1652a45853ebccd3b1320df8cb230c737d0171505c2e59d0f75a7651ee45` |
| AmazonCognito, us-east-1 | 20261020233814 | 2026-10-20T23:38:14Z | `462b0f6b85b152fec36068ece19be65281135619cfafdca1b4195d5938ffd322` |
| AmazonCognito, eu-central-1 | 20261020233814 | 2026-10-20T23:38:14Z | `cb9502edd51a0c76b03322ebcc8b0c3a5c2ae6159d696e723febda22cafaa369` |
| AmazonCognito, me-central-1 | 20261020233814 | 2026-10-20T23:38:14Z | `707d1a62cae32a9ccfdd9625fb529e2faf6448a4c04ece97b6d7cad9eb02883d` |
| AmazonLightsail, us-east-1 | 20260915151951 | 2026-09-15T15:19:51Z | `1ee191ee7e7cd63a41429efb0503dbc12bfdae5db12fccf76db961c929b9dd4b` |
| AmazonLightsail, eu-central-1 | 20260915151951 | 2026-09-15T15:19:51Z | `c927bfc66d7a26e04c02c2c564f5cc026ac292d221733799aef5a4346d5f8414` |

Selected on-demand dimensions. The rate code is `SKU.offerTerm.rateCode`.

Selection rules:

- **Fargate:** ARM vCPU-hours and GB-hours.
- **RDS:** PostgreSQL `db.t4g.small`, single-zone `InstanceUsage` and `Multi-AZUsage`.
- **Storage:** gp3 GB-month for PostgreSQL.
- **ALB:** `LoadBalancerUsage` hours and used `LCUUsage` (not Outposts or reserved).
- **IPv4:** in-use public address hours.
- **S3:** Standard storage, first 50 TB.
- **Cognito:** Essentials MAU.
- **Lightsail:** Linux `BundleUsage:4GB` and `BundleUsage:8GB`, with IPv4.

| Region | Dimension | Rate code | Unit | USD | Term effective |
|---|---|---|---|---|---|
| us-east-1 | Fargate ARM vCPU | `XSZATS4VYMDC9CYN.JRTCKXETXF.6YS6EN2CT7` | hours | 0.03238 | 2026-10-01 |
| us-east-1 | Fargate ARM memory | `UNH9KPQP7W7C66C9.JRTCKXETXF.6YS6EN2CT7` | hours | 0.00356 | 2026-10-01 |
| us-east-1 | RDS single zone | `S9H3AXMBHSKRFJTA.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.032 | 2026-10-01 |
| us-east-1 | RDS Multi-AZ | `5SY529FVXY3EARD8.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.065 | 2026-10-01 |
| us-east-1 | gp3 single zone | `KYVYY29G957PKY3B.JRTCKXETXF.6YS6EN2CT7` | GB-Mo | 0.115 | 2026-10-01 |
| us-east-1 | gp3 Multi-AZ | `J7S7KD4WFDNQWKNX.JRTCKXETXF.6YS6EN2CT7` | GB-Mo | 0.23 | 2026-10-01 |
| us-east-1 | ALB hours | `37CUWUT8GSNQEPUV.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.0225 | 2026-08-01 |
| us-east-1 | ALB LCU | `P2XGEJ8N3KU52WA8.JRTCKXETXF.6YS6EN2CT7` | LCU-Hrs | 0.008 | 2026-08-01 |
| us-east-1 | Public IPv4 | `4GQUNXTFWVSGPUZK.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.005 | 2026-10-01 |
| us-east-1 | S3 Standard | `WP9ANXZGBYYSGJEA.JRTCKXETXF.PGHJ3S3EYE` | GB-Mo | 0.023 | 2026-10-01 |
| us-east-1 | Cognito Essentials | `XVYWD6TWRYVQWN85.JRTCKXETXF.6YS6EN2CT7` | MAU | 0.015 | 2026-09-01 |
| us-east-1 | Lightsail 4 GB | `5EPB2V8S2JCDVZF9.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.03225 | 2026-09-01 |
| us-east-1 | Lightsail 8 GB | `FUZFBMNEZVN47WEC.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.05913 | 2026-09-01 |
| eu-central-1 | Fargate ARM vCPU | `9DKFRNBC4PAQNE7S.JRTCKXETXF.6YS6EN2CT7` | hours | 0.03725 | 2026-10-01 |
| eu-central-1 | Fargate ARM memory | `N789HAYSJQFYU9GM.JRTCKXETXF.6YS6EN2CT7` | hours | 0.00409 | 2026-10-01 |
| eu-central-1 | RDS single zone | `NGGDEX2H83AGNVX3.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.037 | 2026-10-01 |
| eu-central-1 | RDS Multi-AZ | `FUFQ92X3CPTNDE4G.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.074 | 2026-10-01 |
| eu-central-1 | gp3 single zone | `JN8MTUNBMV3GFTFY.JRTCKXETXF.6YS6EN2CT7` | GB-Mo | 0.137 | 2026-10-01 |
| eu-central-1 | gp3 Multi-AZ | `6TQECZDJQGPDX8VF.JRTCKXETXF.6YS6EN2CT7` | GB-Mo | 0.274 | 2026-10-01 |
| eu-central-1 | ALB hours | `UZQQFKE5Z4MCF9D5.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.027 | 2026-08-01 |
| eu-central-1 | ALB LCU | `9H4URPBEA563JK89.JRTCKXETXF.6YS6EN2CT7` | LCU-Hrs | 0.008 | 2026-08-01 |
| eu-central-1 | Public IPv4 | `P6D6UQPC87KFKK3N.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.005 | 2026-10-01 |
| eu-central-1 | S3 Standard | `NRYRNCXF5TWHB476.JRTCKXETXF.PGHJ3S3EYE` | GB-Mo | 0.0245 | 2026-10-01 |
| eu-central-1 | Cognito Essentials | `Y8KDQDK39UDRG6XJ.JRTCKXETXF.6YS6EN2CT7` | MAU | 0.015 | 2026-09-01 |
| eu-central-1 | Lightsail 4 GB | `7SA8SVMGN5Z33DGU.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.03225 | 2026-09-01 |
| eu-central-1 | Lightsail 8 GB | `2FUKKSANGSV4JPKM.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.05913 | 2026-09-01 |
| me-central-1 | Fargate ARM vCPU | `5FBSU6FM2BYRR3RH.JRTCKXETXF.6YS6EN2CT7` | hours | 0.04208 | 2026-10-01 |
| me-central-1 | Fargate ARM memory | `Z7YTUHD92H4DA7M8.JRTCKXETXF.6YS6EN2CT7` | hours | 0.00464 | 2026-10-01 |
| me-central-1 | RDS single zone | `JR8PJ65JK7DS8HN3.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.038 | 2026-10-01 |
| me-central-1 | RDS Multi-AZ | `9B9BQ76K7VGJEBMM.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.076 | 2026-10-01 |
| me-central-1 | gp3 single zone | `5N2AEDAGQKKCRU8R.JRTCKXETXF.6YS6EN2CT7` | GB-Mo | 0.14 | 2026-10-01 |
| me-central-1 | gp3 Multi-AZ | `M97G3X89A4BKXKGZ.JRTCKXETXF.6YS6EN2CT7` | GB-Mo | 0.279 | 2026-10-01 |
| me-central-1 | ALB hours | `JQZCJZDX9TW2VBKB.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.028224 | 2026-08-01 |
| me-central-1 | ALB LCU | `N6YRE5J98AK6MEZK.JRTCKXETXF.6YS6EN2CT7` | LCU-Hrs | 0.00896 | 2026-08-01 |
| me-central-1 | Public IPv4 | `WW37AMRQJBTQ2YR6.JRTCKXETXF.6YS6EN2CT7` | Hrs | 0.005 | 2026-10-01 |
| me-central-1 | S3 Standard | `AWMU7HFPXR55W8U4.JRTCKXETXF.PGHJ3S3EYE` | GB-Mo | 0.025 | 2026-10-01 |
| me-central-1 | Cognito Essentials | `PUSB2GCP8RF2MEMH.JRTCKXETXF.6YS6EN2CT7` | MAU | 0.015 | 2026-09-01 |

## Limits

- The prices are list prices on one date. They change, and the estimate leaves out items listed above.
- The identity options were read in vendor and standards documentation; none was built or tested here.
- The example targets are prompts for a decision, not commitments.
