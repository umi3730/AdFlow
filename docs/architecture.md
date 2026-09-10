# AdFlow System Architecture

## System Overview

AdFlow is a real-time advertising decision platform built as a modular monolith in Go. It handles campaign configuration, user profiling, ad selection through deterministic targeting and auctions, event tracking, and delivery metrics.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Client Layer                                 │
│  (Web Console / External Systems / Load Balancer)                   │
└────────────────────────────────┬────────────────────────────────────┘
                                 │ HTTP/REST
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│                      AdFlow API Server (Go)                          │
│                                                                       │
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │                   HTTP Transport Layer (Gin)                  │  │
│  │  • Authentication (JWT)                                       │  │
│  │  • Request validation                                         │  │
│  │  • Metrics middleware (Prometheus)                            │  │
│  │  • Access logging                                             │  │
│  └──────────────────────────────────────────────────────────────┘  │
│                                 │                                    │
│  ┌──────────────────────────────┼────────────────────────────────┐ │
│  │         Application Services (Business Logic)                 │ │
│  │                              │                                 │ │
│  │  ┌───────────┐  ┌──────────┴─────────┐  ┌─────────────────┐ │ │
│  │  │ Campaign  │  │    Decision        │  │   Event         │ │ │
│  │  │ Service   │  │    Service         │  │   Service       │ │ │
│  │  │           │  │  • Targeting eval  │  │  • Impression   │ │ │
│  │  │  • CRUD   │  │  • Auction logic   │  │  • Click        │ │ │
│  │  │  • Status │  │  • Freq/Budget     │  │  • Conversion   │ │ │
│  │  │    FSM    │  │  • Admission ctrl  │  │  • Idempotency  │ │ │
│  │  └─────┬─────┘  └──────────┬─────────┘  └────────┬────────┘ │ │
│  │        │                   │                      │           │ │
│  │  ┌─────┴─────┐  ┌──────────┴─────────┐  ┌────────┴────────┐ │ │
│  │  │ Identity  │  │ Agent Assistant    │  │   Operations    │ │ │
│  │  │ Service   │  │ (AI Rule Gen)      │  │   Service       │ │ │
│  │  └───────────┘  └────────────────────┘  └─────────────────┘ │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                 │                                    │
│  ┌──────────────────────────────┼────────────────────────────────┐ │
│  │              Domain Layer (Business Rules)                     │ │
│  │   • Campaign aggregate  • Decision logic  • Event validation  │ │
│  │   • Targeting evaluator • Pricing models  • Audit policies    │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                                 │                                    │
│  ┌──────────────────────────────┼────────────────────────────────┐ │
│  │              Adapter Layer (Ports & Adapters)                  │ │
│  │                              │                                 │ │
│  │  ┌──────────┐  ┌─────────────┴────────┐  ┌────────────────┐  │ │
│  │  │  MySQL   │  │      Redis           │  │     Kafka      │  │ │
│  │  │ Adapter  │  │     Adapter          │  │    Adapter     │  │
│  │  │          │  │  • Rate limiter      │  │  • Producer    │  │
│  │  │ • Repos  │  │  • Freq/Budget gate  │  │  • Consumer    │  │
│  │  │ • Outbox │  │  • Profile cache     │  │  • DLQ         │  │
│  │  └────┬─────┘  └──────────┬───────────┘  └────────┬───────┘  │ │
│  │       │                   │                       │           │ │
│  │  ┌────┴─────┐  ┌──────────┴───────────┐  ┌───────┴────────┐ │ │
│  │  │  Memory  │  │   OpenAI-compatible  │  │   HTTP Client  │ │ │
│  │  │ Adapter  │  │      Adapter         │  │    (Agent)     │ │ │
│  │  │ (Local)  │  │   (Agent Provider)   │  │                │ │ │
│  │  └──────────┘  └──────────────────────┘  └────────────────┘ │ │
│  └────────────────────────────────────────────────────────────────┘ │
│                                                                       │
│  ┌────────────────────────────────────────────────────────────────┐ │
│  │              Platform Services                                  │ │
│  │  • Health checks  • Observability (Prometheus)  • pprof        │ │
│  │  • DB pool monitor  • Structured logging  • Graceful shutdown  │ │
│  └────────────────────────────────────────────────────────────────┘ │
└───────────────────────────────────────────────────────────────────────┘
                                 │
                    ┌────────────┼────────────┐
                    ▼            ▼            ▼
         ┌──────────────┐  ┌─────────┐  ┌──────────────┐
         │    MySQL     │  │  Redis  │  │    Kafka     │
         │              │  │         │  │              │
         │ • Campaigns  │  │ • Limit │  │ • Events     │
         │ • Decisions  │  │ • Cache │  │ • Groups     │
         │ • Events     │  │ • Locks │  │ • Dead Letter│
         │ • Profiles   │  └─────────┘  └──────────────┘
         │ • Outbox     │
         │ • Audit      │
         └──────────────┘
```

## Component Responsibilities

### HTTP Transport Layer
- **Gin router** with middleware chain
- **Authentication:** Optional JWT validation with RBAC
- **Metrics:** Prometheus instrumentation for all endpoints
- **Validation:** Request binding and validation
- **Logging:** Structured access logs with request IDs

### Application Services

#### Campaign Service
- Campaign CRUD operations
- State machine (Draft → Published → Paused → Resumed)
- Optimistic concurrency control (version-based)
- Creative management

#### Decision Service
- **Admission control:** Token bucket (local) or sliding window (Redis)
- **Concurrency control:** Semaphore-based max in-flight requests
- **Decision logic:** Candidate filtering, targeting evaluation, auction
- **Resource reservation:** Separate atomic Redis Lua reservations for frequency and budget; failed later steps release resources already acquired
- **Idempotency:** MySQL-based request deduplication
- **Caching:** 5s immutable candidate snapshot, profile cache-aside

#### Event Service
- **Synchronous mode:** Direct persistence and settlement
- **Asynchronous mode:** Transactional outbox → Kafka → Consumer
- Impression/click/conversion tracking with validation
- Idempotent event processing (`eventId` deduplication)

#### Identity Service
- JWT generation and validation
- Bcrypt password hashing
- Hierarchical RBAC (viewer/operator/admin)

#### Agent Assistant Service
- AI-powered campaign rule generation
- OpenAI-compatible provider abstraction
- Circuit breaker with mock fallback
- Safety limits and validation

#### Operations Service
- Outbox inspection and statistics
- Kafka consumer lag monitoring
- Request tracing (decision → settlement → publication)
- Dead letter replay (admin only)

### Domain Layer
- **Pure business logic** with no external dependencies
- **Targeting evaluator:** Deterministic all/any/none rule matching
- **Auction logic:** First-price and fixed-cost pricing
- **Frequency/Budget models:** TTL-based reservations
- **Campaign aggregate:** Immutable published versions

### Adapter Layer
- **MySQL:** Campaigns, decisions, events, profiles, audit, outbox
- **Redis:** Rate limiting, frequency/budget gates, profile cache
- **Kafka:** Event publication and consumption
- **Memory:** Local development adapters (no infrastructure needed)
- **OpenAI:** Remote LLM provider for Agent assistant

### Platform Services
- **Health checks:** Liveness + readiness with dependency checks
- **Observability:** Prometheus metrics, pprof profiling
- **Database monitoring:** Connection pool statistics
- **Graceful shutdown:** Context cancellation with timeout

## Data Flow Patterns

### Write Path (Campaign)
```
HTTP POST → Validation → Service → Domain Logic → MySQL Repository → HTTP 201
```

### Read Path (Campaign)
```
HTTP GET → Service → MySQL Repository → Domain Model → HTTP 200
```

### Decision Path (Hot Path)
```
HTTP POST → Admission Control → Rate Limit Check
  → Profile Lookup (Redis/MySQL cache-aside)
  → Candidate Loading (5s local cache)
  → Targeting Evaluation (in-memory)
  → Frequency Reservation → Budget Reservation (each atomic; cleanup on failure)
  → Decision Persistence (MySQL)
  → HTTP 200
```

### Event Path (Async Mode)
```
HTTP POST → Validation → Outbox Write (MySQL transaction)
  → HTTP 202 Accepted
  → [Background] Settlement Worker → Redis Confirm → Mark SETTLED
  → [Background] Relay → Kafka Publish → Mark PUBLISHED
  → [Background] Consumer → Event Persistence → Metrics Update
```

## Deployment Architecture

### Single Instance (Local Development)
```
API Server (in-memory adapters)
  → No external dependencies
  → Fast iteration
```

### Possible Multi-Instance Deployment (Not a Validated Topology)

The following is an extension sketch. Local capacity tests used shared local infrastructure and a single-replica Kafka topic; they do not validate MySQL replication, Redis Cluster, or multi-replica failover. Redis multi-key scripts need a compatible hash-slot/key design before a Cluster migration.
```
              Load Balancer
                    │
        ┌───────────┼───────────┐
        ▼           ▼           ▼
      API-1       API-2       API-3
        │           │           │
        └───────────┼───────────┘
                    │
    ┌───────────────┼───────────────┐
    ▼               ▼               ▼
  MySQL          Redis           Kafka
(Main/Replica)  (Cluster)      (Cluster)
```

## Technology Stack

- **Language:** Go 1.27
- **HTTP Framework:** Gin
- **Database:** MySQL 8.0 (with sql.DB connection pooling; queue claims use `FOR UPDATE SKIP LOCKED`)
- **Cache:** Redis 6+
- **Message Queue:** Kafka (franz-go client)
- **Metrics:** Prometheus
- **Profiling:** pprof
- **Authentication:** JWT (HS256)
- **Logging:** slog (structured JSON)

## Key Design Decisions

### Modular Monolith
- **Why:** Simpler deployment, easier development, sufficient for current scale
- **Trade-off:** All contexts in one process (easier to debug, harder to scale independently)

### Hexagonal Architecture
- **Why:** Clean separation, testability, adapter swapping
- **Ports:** Domain interfaces (Repository, Provider, Gate, Store)
- **Adapters:** MySQL, Redis, Kafka, Memory, HTTP

### Optimistic Concurrency
- **Why:** Detect stale administrative writes instead of silently overwriting another update
- **Implementation:** Campaign `revision` checked by conditional SQL updates; published rule `version` is a separate historical identifier

### Transactional Outbox
- **Why:** At-least-once event delivery without distributed transactions
- **Implementation:** In Kafka mode, event receipts, settlement tasks and Outbox events are accepted in one MySQL transaction. This does not make Redis and MySQL a single transaction, or make every campaign domain event an Outbox message.
- **Relay:** Publication waits for settlement proof; consumers validate accepted payloads and process events idempotently

### Immutable Candidate Cache
- **Why:** Reduce database load on hot path
- **TTL:** 5 seconds (acceptable eventual consistency for campaigns)
- **Invalidation:** Time-based only (no active invalidation)

### Profile Cache-Aside
- **Why:** Reduce profile database reads (high read/write ratio)
- **Implementation:** Redis with MySQL fallback
- **Negative caching:** 5s TTL for missing profiles
- **Updates:** Persist to MySQL, then invalidate Redis with a new random generation; Lua only permits matching-generation refills. Failed invalidation remains a bounded consistency/recovery concern.

### Atomic Reservations
- **Why:** Prevent over-spending and frequency cap violations
- **Implementation:** Separate atomic frequency and budget reservations, with cleanup after a later reservation fails. Exposure settlement confirms both reservations and writes an idempotency receipt in one Lua operation.
- **TTL:** 30s reservation window

### Settlement Recovery
- **Worker:** Claims at most 4 tasks with a 15s lease and processes them sequentially; failed iterations return unfinished claims using an owner-checked, bounded cleanup context
- **Transaction retries:** Retry MySQL 1213/1205 only after explicit whole-transaction rollback; a lost commit acknowledgement is not blindly retried
- **Uncertain state:** Missing/expired reservations without valid proof enter `RECONCILE`; this is not an unlimited automatic recovery guarantee

## Scalability Considerations

These are extension directions unless explicitly identified as implemented. They are not additional performance results or a claim that all pictured infrastructure is already deployed.

### Horizontal Scaling
- **Stateless API instances** behind load balancer
- **Shared state** in MySQL/Redis/Kafka
- **Connection pooling** to prevent database overload
- **Rate limiting** at application level (Redis-based shared window)

### Database Scaling
- **Read replicas** for read-heavy queries (profiles, metrics)
- **Connection pool:** default 30 max / 10 idle; limits configurable and covered by the 30/60/100 comparison
- **Indexes** on hot paths (decision lookup, outbox claiming)

### Cache Scaling
- **Redis Cluster** requires adapting and validating multi-key script placement; current reservation/cache adapters use a single Redis client
- **TTL-based eviction** to prevent unbounded growth
- **Profile cache failure:** Bounded Redis calls fall back to MySQL; budget/frequency operations must not bypass their gate. The Agent model circuit breaker is a separate mechanism.

### Queue Scaling
- **Kafka partitioning** by requestId for ordering
- **Consumer group** for parallel consumption
- **Backpressure gate** to prevent overload from async pipeline

## Observability

### Metrics (Prometheus)
- HTTP request rate, latency, errors
- Decision outcomes, admission results
- Database connection pool statistics
- Cache hit rates
- Kafka consumer lag
- Outbox depth by status

### Logging (slog)
- Structured JSON logs
- Request IDs for tracing
- Error context
- Business events (campaign published, decision made)

### Profiling (pprof)
- CPU profiling for hot path optimization
- Memory profiling for allocation analysis
- Goroutine profiling for concurrency debugging
- Profiling is disabled by default; explicitly configure a distinct loopback port per API instance

## Security

- **Authentication:** Optional JWT with configurable issuers
- **Authorization:** Role-based (viewer/operator/admin)
- **Rate limiting:** Prevent abuse and DoS
- **Input validation:** Gin binding with struct tags
- **SQL injection:** Parameterized queries only
- **Secrets:** Environment variables, never in code
- **Audit trail:** Append-only audit log for sensitive actions

---

**Last updated:** 2026-09-10
