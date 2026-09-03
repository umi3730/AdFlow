# AdFlow

AdFlow is a Go-based real-time advertising decision platform built as a portfolio-grade backend project. The MVP covers campaign configuration, deterministic targeting, frequency caps, budget reservations, ad events, and an approval-gated Agent rule assistant.

## Current milestone

M0 foundation:

- Go modular monolith layout with Gin at the HTTP boundary
- JSON structured logging
- liveness and dependency-aware readiness endpoints
- request ID, structured access logging, recovery, and validated JSON binding
- MySQL and Redis clients with bounded health checks
- graceful shutdown
- initial campaign schema
- dependency-free in-memory development mode

M1 campaign context (implemented):

- DDD-lite Campaign aggregate and value objects
- draft, publish, pause, and resume state transitions
- immutable active campaign version
- optimistic revision checks in the Repository port
- in-memory Repository adapter for local development and tests
- Gin REST endpoints with end-to-end lifecycle tests

M2 decision context (implemented):

- profile tags and fields
- deterministic all/any/none targeting evaluation
- active campaign and creative candidate adapter
- request idempotency
- atomic in-memory frequency and budget reservations with TTL release
- deterministic creative selection
- matched and explicit No-Ad responses
- selectable per-process token-bucket or Redis shared sliding-window request admission
- bounded in-flight decision execution and short queue timeout
- per-request processing deadline and fail-fast 429/503/504 responses
- cancellation-safe release of frequency and budget reservations

M3 measurement context (implemented with local and Kafka modes):

- Redis Lua frequency and budget reservation adapter
- reservation confirmation and release
- idempotent impression, click, and conversion events
- impression-before-click/conversion ordering rule
- campaign delivery metrics
- Kafka producer and consumer group adapters using franz-go
- versioned `adflow.ad-events.v1` event envelope
- `requestId` partition key for per-decision ordering
- MySQL transactional outbox with duplicate receipt protection
- lease-based multi-relay batch claiming compatible with MySQL 5.7
- exponential publish retry and consumer-side persistent `eventId` idempotency
- dead-letter publication after eight failed relay attempts
- persistent decision idempotency records for delayed event verification
- Prometheus outbox-depth, relay-result, and Kafka consumer-lag metrics

M5 Agent assistant (implemented with mock and remote-provider modes):

- provider-neutral rule generation interface
- deterministic local mock provider for offline development
- OpenAI Responses API mode with strict JSON Schema Structured Outputs
- OpenAI-compatible Chat Completions mode for providers that expose JSON Object output
- bounded whole-operation timeout, retryable-status backoff, and redirect protection
- circuit breaker with a single half-open probe and optional local Mock fallback
- model request ID, token usage, latency, fallback, model, and prompt-version metadata
- Prometheus generation, duration, token, and circuit-state metrics
- opt-in live evaluation set including budget escalation and prompt-injection cases
- campaign-domain validation of every generated draft
- explicit provider, model, prompt-version, explanation, and warning metadata
- separate human confirmation before campaign publication

M4 administration UI (basic implementation):

- React and TypeScript working console
- campaign, creative, profile, decision, event, and metric workflows
- Agent rule draft preview and explicit human-confirmed publication
- responsive navigation, API connection state, and failure feedback

M6 identity and audit context (implemented):

- optional HS256 JWT authentication with issuer and expiry validation
- hierarchical RBAC roles: viewer, operator, and admin
- admin-only campaign publication and audit-log access
- append-only business-action audit records with success/failure outcomes
- in-memory and MySQL audit-store adapters
- local development identities without changing the default frontend workflow

## Run locally

Prerequisites: Go 1.27+. MySQL 5.7+ and Redis 6+ are optional while using the in-memory adapters.

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

Event processing defaults to synchronous local mode. To enable Kafka, create the configured topic and set:

```env
ADFLOW_EVENT_TRANSPORT=kafka
ADFLOW_DECISION_STORE=mysql
ADFLOW_PROFILE_STORE=mysql
ADFLOW_KAFKA_BROKERS=127.0.0.1:9092
ADFLOW_KAFKA_TOPIC=adflow.ad-events.v1
ADFLOW_KAFKA_DEAD_LETTER_TOPIC=adflow.ad-events.dlq.v1
ADFLOW_KAFKA_CONSUMER_GROUP=adflow-metrics-v1
```

Kafka uses `requestId` as the record key, at-least-once delivery, manual offset commits after successful processing, and `eventId` idempotency at the consumer boundary. Kafka mode requires migrations `000002_events` and `000003_decisions`.

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
ADFLOW_DECISION_BURST=1000
ADFLOW_DECISION_MAX_IN_FLIGHT=256
ADFLOW_DECISION_QUEUE_TIMEOUT=5ms
ADFLOW_DECISION_TIMEOUT=100ms
```

`memory` uses a process-local token bucket and its burst setting. Set `ADFLOW_DECISION_RATE_LIMITER=redis` to enforce a shared exact sliding window across API replicas. The Redis adapter uses one Lua script to remove expired ZSET members, count the active window, append the current request, and refresh the key TTL. Redis server time avoids application-host clock skew; a limiter dependency error fails closed with HTTP 503.

Both adapters reject excess arrival rate with HTTP 429. The per-process concurrency gate remains in place because it protects each replica's CPU, database pool, and downstream dependencies independently of the shared rate limit. It waits only for the configured queue timeout before returning HTTP 503. Accepted execution receives its own processing deadline and returns HTTP 504 if it expires. Prometheus exposes admission outcomes, queue duration, current decision concurrency, and execution timeouts.

The Agent assistant defaults to the deterministic Mock provider. To use an OpenAI Responses-compatible endpoint:

```env
ADFLOW_AGENT_PROVIDER=openai-compatible
ADFLOW_AGENT_BASE_URL=https://api.openai.com/v1
ADFLOW_AGENT_API_KEY=replace-with-a-runtime-secret
ADFLOW_AGENT_MODEL=replace-with-a-supported-model
ADFLOW_AGENT_API_STYLE=responses
```

For another provider exposing OpenAI-compatible Chat Completions, set its documented base URL and model, then use `ADFLOW_AGENT_API_STYLE=chat_completions`. Responses mode sends a strict JSON Schema. Compatibility mode requests a JSON object and includes the contract in the system instruction; both modes pass through the same strict JSON decoder, Agent safety limits, campaign-domain validation, human confirmation, RBAC, and audit trail.

Provider calls have an 8-second whole-operation timeout, at most two retries by default, and a circuit that opens after three failed calls. With fallback enabled, an unavailable provider returns a clearly marked local Mock draft instead of publishing or silently inventing a remote result. API keys are read only from the environment and are never included in logs or API responses.

Live evaluations are opt-in because they call the configured paid provider:

```powershell
$env:ADFLOW_AGENT_RUN_LIVE_EVALS = "true"
go test -run TestLiveProviderEvaluation -v ./internal/agentassistant/adapter/openai
```

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
- `POST /v1/agent/rule-drafts` — generate and validate a targeting rule draft without publishing it

## Architecture direction

The first release is a modular monolith. Gin is restricted to the HTTP transport layer; domain services use standard `context.Context` and depend on interfaces. MySQL, Redis, HTTP, and future model providers live behind adapters. Services will be split only after load tests identify a concrete scaling or isolation need.

## Next milestone

The real MySQL/Redis/Kafka correctness baseline is implemented. The next backend milestone is sustained load and chaos testing with production-shaped data, followed by query-plan and index regression evidence. The local synchronous path remains the default, and authentication remains opt-in until the administration UI gains a login flow.
