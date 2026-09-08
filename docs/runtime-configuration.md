# 运行配置与接口参考

## Run locally

Prerequisites: Go 1.27+. MySQL 8.0+ and Redis 6+ are optional while using the in-memory adapters.

```powershell
go mod download
go run ./cmd/api
```

Campaign persistence defaults to the in-memory adapter so the API can be explored without MySQL. Set `ADFLOW_CAMPAIGN_REPOSITORY=mysql` after applying the migration to use the MySQL adapter.

Apply all pending MySQL migrations with the repeatable migration command:

```powershell
$env:ADFLOW_MYSQL_DSN = "adflow:adflow@tcp(127.0.0.1:3306)/adflow?parseTime=true&charset=utf8mb4&loc=UTC"
go run ./cmd/migrate -dir migrations
```

The runner serializes concurrent migration attempts with a MySQL advisory lock and records each file checksum in `schema_migrations`. It refuses to continue if an already-applied migration file changes.

Frequency and budget reservations also default to memory. Set `ADFLOW_RESERVATION_ADAPTER=redis` to use the atomic Redis Lua adapter.

For this workstation's native WSL setup, `ops/native/start-components.ps1` exposes MySQL on `127.0.0.1:13306` to avoid the unrelated Windows MySQL service on 3306. `ops/native/start-api.ps1 -Port 18080` builds and starts the API using the local `.env`. See [the verified local async cutover](async-switch-20260907.md).

Event processing defaults to synchronous local mode. To enable Kafka, create the configured topic and set:

```env
ADFLOW_EVENT_TRANSPORT=kafka
ADFLOW_DECISION_STORE=mysql
ADFLOW_RESERVATION_ADAPTER=redis
ADFLOW_PROFILE_STORE=mysql-redis
ADFLOW_PROFILE_CACHE_TTL=5m
ADFLOW_PROFILE_NEGATIVE_CACHE_TTL=5s
ADFLOW_PROFILE_CACHE_TIMEOUT=10ms
ADFLOW_KAFKA_BROKERS=127.0.0.1:9092
ADFLOW_KAFKA_TOPIC=adflow.ad-events.v1
ADFLOW_KAFKA_DEAD_LETTER_TOPIC=adflow.ad-events.dlq.v1
ADFLOW_KAFKA_CONSUMER_GROUP=adflow-metrics-v1
```

Kafka uses `requestId` as the record key, at-least-once delivery, manual offset commits after successful processing, and `eventId` idempotency at the consumer boundary. Apply all migrations through `000013_auth_users` on MySQL 8. Kafka mode requires MySQL decisions and Redis reservations so another worker can recover accepted events after a process restart. Profile IDs are case-sensitive; new impressions require an unexpired decision, while clicks and conversions use a seven-day window from the original impression's server acceptance. See [the core correctness fixes](core-fixes-20260907.md).

Impressions enter `SETTLING` after durable acceptance. A background worker atomically confirms the winning price and frequency in Redis, then releases the request's events for publication. Missing reservation proof enters `RECONCILE`; these events cannot be published or counted. Before upgrading, stop old API, relay and consumer processes; the migration quarantines unprocessed legacy events whose settlement cannot be proven. See [settlement recovery and verification](outbox-settlement-fix-20260907.md).

`mysql-redis` keeps MySQL as the profile source of truth. Writes and deletions atomically rotate a Redis generation and invalidate cached data; source reads may refill only the generation they started with. This prevents stale cross-instance queries and late writer completions from repopulating old values. Missing users receive short negative-cache entries. Redis operations have a small independent timeout and reads fall back to MySQL; failed invalidation after a persistent write is surfaced for retry. This does not make the MySQL/Redis mutation crash-atomic. See [onboarding and cache verification](onboarding-cache-improvements-20260907.md).

The local API listens on `http://localhost:18080` by default; the administration UI runs on `http://localhost:3000`.

Authentication is disabled by default so the local UI remains frictionless. Enable it with:

```env
ADFLOW_AUTH_ENABLED=true
ADFLOW_JWT_SECRET=replace-with-at-least-32-random-characters
ADFLOW_ACCESS_TOKEN_TTL=30m
```

Local and test environments provide three demonstration accounts when `ADFLOW_AUTH_USERS` is empty:

| Username | Password | Role | Permissions |
| --- | --- | --- | --- |
| `viewer` | `adflow-viewer` | viewer | Read APIs |
| `operator` | `adflow-operator` | operator | Read, configure, and run debugging workflows |
| `admin` | `adflow-admin` | admin | Operator permissions, campaign publication, and audit-log access |

Outside local/test environments, enabled authentication requires `ADFLOW_AUTH_USERS`. Its format is a semicolon-separated list of `username:role:bcryptHash` entries. Never store plaintext production passwords in this value. Set `ADFLOW_AUDIT_STORE=mysql` after applying migration `000005_identity_audit` to persist the audit trail.

The decision endpoint protects the hot path with two independent controls:

```env
ADFLOW_DECISION_RATE_LIMIT=5000
ADFLOW_DECISION_RATE_LIMITER=memory
ADFLOW_DECISION_RATE_WINDOW=1s
ADFLOW_CANDIDATE_CACHE_TTL=5s
ADFLOW_DECISION_BURST=1000
ADFLOW_DECISION_MAX_IN_FLIGHT=256
ADFLOW_DECISION_QUEUE_TIMEOUT=5ms
ADFLOW_DECISION_TIMEOUT=100ms
```

`memory` uses a process-local token bucket and its burst setting. Set `ADFLOW_DECISION_RATE_LIMITER=redis` to enforce a shared exact sliding window across API replicas. The Redis adapter uses one Lua script to remove expired ZSET members, count the active window, append the current request, and refresh the key TTL. Redis server time avoids application-host clock skew; a limiter dependency error fails closed with HTTP 503.

Active campaign and creative candidates are held in a per-process immutable snapshot keyed by slot. The default five-second TTL bounds configuration propagation delay, while concurrent cache misses are coalesced into one MySQL refresh. Prometheus exposes cache hit, miss, shared-wait, error, and refresh-duration metrics. Profile, reservation, budget, and decision-idempotency state remain request-specific and are never stored in this snapshot.

Both adapters reject excess arrival rate with HTTP 429. The per-process concurrency gate remains in place because it protects each replica's CPU, database pool, and downstream dependencies independently of the shared rate limit. It waits only for the configured queue timeout before returning HTTP 503. Accepted execution receives its own processing deadline and returns HTTP 504 if it expires. Prometheus exposes admission outcomes, queue duration, current decision concurrency, and execution timeouts.

The Agent assistant defaults to the deterministic Mock provider. To use an OpenAI Responses-compatible endpoint:

```env
ADFLOW_AGENT_PROVIDER=openai-compatible
ADFLOW_AGENT_BASE_URL=https://api.openai.com/v1
ADFLOW_AGENT_API_KEY=replace-with-a-runtime-secret
ADFLOW_AGENT_MODEL=replace-with-a-supported-model
ADFLOW_AGENT_API_STYLE=responses
ADFLOW_AGENT_THINKING=
```

For another provider exposing OpenAI-compatible Chat Completions, set its documented base URL and model, then use `ADFLOW_AGENT_API_STYLE=chat_completions`. `ADFLOW_AGENT_THINKING` is optional; set it to `enabled` or `disabled` only when the provider supports that request extension. Responses mode sends a strict JSON Schema. Compatibility mode requests a JSON object and includes the contract in the system instruction; both modes pass through the same strict JSON decoder, Agent safety limits, campaign-domain validation, human confirmation, RBAC, and audit trail.

Provider calls have an 8-second whole-operation timeout, at most two retries by default, and a circuit that opens after three failed calls. With fallback enabled, an unavailable provider returns a clearly marked local Mock draft instead of publishing or silently inventing a remote result. API keys are read only from the environment and are never included in logs or API responses.

Live evaluations are opt-in because they call the configured paid provider:

```powershell
$env:ADFLOW_AGENT_RUN_LIVE_EVALS = "true"
go test -run TestLiveProviderEvaluation -v ./internal/agentassistant/adapter/openai
```

The first recorded live run uses DeepSeek V4 Flash through Chat Completions with thinking disabled. See `docs/agent-live-evaluation.md` for the failed compatibility attempts, prompt-contract correction, final four-case pass, and HTTP end-to-end evidence. A local credential may be kept in the ignored `.env` file, but it must be loaded into the process environment before starting AdFlow; the application does not automatically parse dotenv files.

Run the administration UI in a second terminal:

```powershell
cd web
npm run dev
```

The current UI includes dashboard metrics, campaign lifecycle actions, creative management, simulated profiles, decision debugging, and impression/click/conversion controls.

The OpenAPI 3.1 contract is maintained at `docs/openapi.yaml`.

Run a decision load test after installing k6:

```powershell
k6 run tests/load/decision.js
```

For a repeatable full-path baseline with configurable campaign/profile cardinality, a warm-up phase, steady arrival rate, and impression ingestion, run:

```powershell
$env:CAMPAIGN_COUNT = "20"
$env:PROFILE_COUNT = "1000"
$env:TARGET_RATE = "200"
$env:WARMUP_DURATION = "30s"
$env:STEADY_DURATION = "2m"
$env:COOLDOWN_DURATION = "15s"
New-Item -ItemType Directory -Force work | Out-Null
k6 run --summary-export=work/k6-delivery.json tests/load/delivery-sustained.js
```

The script creates one matching campaign segment per campaign, distributes a reusable profile pool across those segments, then measures decision latency and the complete decision-to-impression path. Use `SEND_IMPRESSIONS=false` to isolate decision performance. Setup traffic has named tags so it can be separated from steady-state endpoint metrics.
Set a stable `RUN_ID`, then use `SEED_DATA=false` on later runs to reuse the same campaigns and profiles for comparable A/B measurements.

To demonstrate overload behavior, lower the admission limits and run the arrival-rate spike profile:

```powershell
$env:ADFLOW_DECISION_RATE_LIMIT = "200"
$env:ADFLOW_DECISION_BURST = "50"
go run ./cmd/api

k6 run tests/load/decision-spike.js
```

HTTP 429 and 503 are expected backpressure in this profile; HTTP 500 remains a failure.

The script reports decision-specific P95/P99 latency and error rate. Keep the machine configuration and test parameters with any resume performance numbers.

## Real infrastructure verification

The default test suite remains self-contained. Real MySQL, Redis, and Kafka checks are opt-in through the `integration` build tag and environment variables; no application container image is required.

```powershell
$env:ADFLOW_IT_MYSQL_DSN = "adflow:password@tcp(127.0.0.1:3306)/adflow_it?parseTime=true&charset=utf8mb4&loc=UTC"
$env:ADFLOW_IT_REDIS_ADDR = "127.0.0.1:6379"
$env:ADFLOW_IT_KAFKA_BROKERS = "127.0.0.1:9092"
$env:ADFLOW_IT_KAFKA_TOPIC = "adflow.it.events.v1"
$env:ADFLOW_IT_KAFKA_DEAD_LETTER_TOPIC = "adflow.it.dlq.v1"
$env:ADFLOW_IT_KAFKA_RESTART_TOPIC = "adflow.it.restart.v1"
go test -tags=integration -v ./tests/integration
```

Create the three Kafka topics before running the suite and apply every migration to the test database. The verification covers MySQL optimistic concurrency and Outbox lease competition, Redis atomic sliding-window/budget admission, duplicate publication after a Kafka acknowledgment, and consumer replay after a side effect but before offset commit. See `docs/integration-verification.md` for the tested failure timelines.

Endpoints:

- `GET /livez` — process liveness
- `GET /readyz` — MySQL and Redis readiness
- `GET /metrics` — Prometheus HTTP, decision, event, process, and Go runtime metrics
- `POST /v1/auth/login` — exchange credentials for a short-lived JWT
- `GET /v1/auth/me` — read the authenticated principal and role
- `GET /v1/audit-logs` — list append-only action records (admin only)
- `POST /v1/campaigns` — create a draft campaign
- `PUT /v1/campaigns/{id}` — edit a draft campaign
- `GET /v1/campaigns` — list campaigns with status/limit/offset filters
- `GET /v1/campaigns/{id}` — fetch a campaign
- `POST /v1/campaigns/{id}/publish` — publish an immutable version
- `POST /v1/campaigns/{id}/pause` — pause an active campaign
- `POST /v1/campaigns/{id}/resume` — resume a paused campaign
- `POST /v1/campaigns/{id}/creatives` — create an active creative
- `GET /v1/campaigns/{id}/creatives` — list campaign creatives
- `POST /v1/campaigns/{id}/creatives/{creativeId}/disable` — disable a creative
- `PUT /v1/profiles/{userId}` — create or replace a simulated user profile
- `POST /v1/decisions` — request an advertisement decision
- `POST /v1/events` — record an impression, click, or conversion
- `GET /v1/campaigns/{id}/metrics` — read campaign delivery metrics
- `GET /v1/operations/outbox` — inspect Outbox records and aggregate status counts
- `GET /v1/operations/kafka-lag` — read latest observed lag by Kafka partition
- `POST /v1/operations/dead-letters/{eventId}/replay` — requeue a dead letter (admin only)
- `POST /v1/agent/rule-drafts` — generate and validate a targeting rule draft without publishing it

## Architecture direction

The first release is a modular monolith. Gin is restricted to the HTTP transport layer; domain services use standard `context.Context` and depend on interfaces. MySQL, Redis, HTTP, and future model providers live behind adapters. Services will be split only after load tests identify a concrete scaling or isolation need.
