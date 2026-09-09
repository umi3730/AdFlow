# AdFlow Request Flow Diagrams

This document illustrates the detailed request flows for key operations in AdFlow.

## 1. Decision Request Flow (Synchronous Mode)

The decision flow is the hot path - optimized for low latency.

```
┌────────┐
│ Client │
└───┬────┘
    │
    │ POST /v1/decisions
    │ {requestId, userId, slotId}
    │
    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                          HTTP Transport Layer                            │
└───────────────────────────────┬─────────────────────────────────────────┘
    │
    │ 1. Authenticate (if enabled)
    │ 2. Validate request
    │ 3. Metrics: increment in_flight
    │
    ▼
┌─────────────────────────────────────────────────────────────────────────┐
│                         Admission Control                                │
└───────────────────────────────┬─────────────────────────────────────────┘
    │
    │ Rate Limiter (Token Bucket / Redis Sliding Window)
    ├─[REJECTED]─→ HTTP 429 Too Many Requests
    │
    │ Concurrency Gate (Semaphore - max in-flight)
    ├─[QUEUE FULL]─→ HTTP 503 Service Unavailable
    │
    ▼ [ADMITTED]
┌─────────────────────────────────────────────────────────────────────────┐
│                          Decision Service                                │
└───────────────────────────────┬─────────────────────────────────────────┘
    │
    │ Check idempotency (MySQL)
    ├─[EXISTS]─→ Return existing decision
    │
    ▼ [NEW REQUEST]
    │
    │ Acquire decision lock (MySQL - prevent duplicate processing)
    ├─[LOCKED]─→ Wait briefly (2ms poll, 250ms max)
    │
    ▼ [ACQUIRED]
    │
    ├──────────────────────────────────────────────────────────┐
    │                                                           │
    ▼ Parallel Operations                                      ▼
┌──────────────────────────┐                    ┌───────────────────────┐
│   Load User Profile      │                    │   Load Candidates     │
│                          │                    │                       │
│ 1. Check Redis cache     │                    │ 1. Check local cache  │
│    ├─[HIT]─→ Return      │                    │    (5s TTL)           │
│    │                     │                    │    ├─[HIT]─→ Return   │
│    └─[MISS]              │                    │    │                  │
│ 2. Query MySQL           │                    │    └─[MISS]          │
│ 3. Store in Redis        │                    │ 2. Query MySQL       │
│    (5min TTL)            │                    │    (campaigns +      │
│                          │                    │     creatives)        │
│ Profile {tags, fields}   │                    │ 3. Update cache      │
└────────────┬─────────────┘                    │                       │
             │                                   │ []Candidate           │
             │                                   └──────────┬────────────┘
             │                                              │
             └──────────────────┬───────────────────────────┘
                                │
                                ▼
                    ┌─────────────────────────┐
                    │  Targeting Evaluation   │
                    │  (In-Memory)            │
                    │                         │
                    │ For each candidate:     │
                    │   Match(profile, rules) │
                    │                         │
                    │ Rank by:                │
                    │ 1. Auction bid (desc)   │
                    │ 2. Priority             │
                    │ 3. Stable hash          │
                    └────────────┬────────────┘
                                 │
                                 ▼
                    ┌─────────────────────────┐
                    │  For top candidate:     │
                    └────────────┬────────────┘
                                 │
        ┌────────────────────────┼────────────────────────┐
        │                        │                        │
        ▼                        ▼                        ▼
┌───────────────┐     ┌──────────────────┐     ┌─────────────────┐
│ Frequency     │     │  Budget          │     │ Creative        │
│ Check         │     │  Check           │     │ Selection       │
│               │     │                  │     │                 │
│ Redis EVAL:   │     │  Redis EVAL:     │     │ Hash(requestId) │
│ - Check count │     │  - Check spend   │     │ → creativeId    │
│ - Reserve     │     │  - Reserve cost  │     │                 │
│ - Set TTL 30s │     │  - Set TTL 30s   │     │                 │
│               │     │                  │     │                 │
│ [CAPPED] ──┐  │     │  [EXHAUSTED] ──┐ │     │                 │
└────────────┼──┘     └─────────────┼──┘       └─────────────────┘
             │                       │
             │ [ALLOWED]             │ [ALLOWED]
             │                       │
             └───────────┬───────────┘
                         │
                         ▼
             ┌─────────────────────────┐
             │  Commit Decision        │
             │  (MySQL)                │
             │                         │
             │  INSERT decision        │
             │  WHERE owner = self     │
             │                         │
             │  Result {               │
             │    matched: true,       │
             │    campaignId,          │
             │    creativeId,          │
             │    price,               │
             │    reservationToken,    │
             │    expiresAt            │
             │  }                      │
             └────────────┬────────────┘
                          │
                          │ Release decision lock
                          │
                          ▼
              ┌──────────────────────┐
              │  HTTP 200 OK         │
              │  + Decision Result   │
              └──────────┬───────────┘
                         │
                         ▼
                   ┌──────────┐
                   │  Client  │
                   └──────────┘

Timeline: ~50-150ms (P95: ~100ms)
- Profile lookup: 1-5ms (Redis) or 10-20ms (MySQL miss)
- Candidate load: <1ms (cached) or 15-30ms (MySQL miss)
- Targeting eval: <1ms (in-memory)
- Frequency/Budget: 2-5ms (Redis Lua)
- Decision commit: 10-20ms (MySQL)
```

## 2. No-Ad Decision Flow

When no matching campaign is found or resources are exhausted:

```
Decision Service
      │
      ├─ No matching candidates
      │  OR
      ├─ All candidates fail targeting
      │  OR
      └─ Frequency/Budget exhausted for all

      ▼
┌──────────────────────────┐
│ Commit No-Ad Decision    │
│                          │
│ Result {                 │
│   matched: false,        │
│   reason: [              │
│     "no_candidate" |     │
│     "targeting_miss" |   │
│     "frequency_capped" | │
│     "budget_exhausted"   │
│   ]                      │
│ }                        │
└────────────┬─────────────┘
             │
             ▼
     HTTP 200 OK (No-Ad)

Timeline: ~20-50ms (faster - no reservations)
```

## 3. Event Ingestion Flow (Async Mode)

Events are accepted immediately and processed asynchronously for high throughput.

```
┌────────┐
│ Client │
└───┬────┘
    │
    │ POST /v1/events
    │ {eventId, type: "impression", requestId, timestamp, ...}
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Event Service                               │
└───────────────────────────────┬─────────────────────────────────┘
    │
    │ 1. Validate event type
    │ 2. Check decision exists + not expired (MySQL)
    ├─[INVALID]─→ HTTP 400 Bad Request
    │
    │ 3. For click/conversion: check impression exists
    ├─[MISSING DEPENDENCY]─→ HTTP 400
    │
    ▼ [VALID]
┌─────────────────────────────────────────────────────────────────┐
│                MySQL Transaction BEGIN                           │
│                                                                  │
│  1. INSERT INTO events                                          │
│     ON DUPLICATE KEY UPDATE (idempotent by eventId)            │
│                                                                  │
│  2. INSERT INTO outbox                                          │
│     status = 'SETTLING'                                         │
│     request_id = event.requestId  (for settlement lookup)      │
│     event_ids = [eventId]                                       │
│                                                                  │
│  COMMIT                                                          │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                                 ▼
                      ┌──────────────────────┐
                      │  HTTP 202 Accepted   │
                      └──────────┬───────────┘
                                 │
                                 ▼
                           ┌──────────┐
                           │  Client  │
                           └──────────┘

                    [Asynchronous Pipeline Below]
                                 │
                                 │
    ┌────────────────────────────┼────────────────────────────┐
    │                            │                            │
    ▼                            ▼                            ▼
┌─────────────────┐   ┌──────────────────┐   ┌────────────────────┐
│ Settlement      │   │  Relay Worker    │   │ Kafka Consumer     │
│ Worker          │   │                  │   │                    │
│                 │   │                  │   │                    │
│ Poll SETTLING   │   │ Poll SETTLED     │   │ Subscribe to topic │
│ rows (batch)    │   │ rows (batch)     │   │                    │
│                 │   │                  │   │                    │
│ For each:       │   │ Publish to Kafka │   │ Consume events     │
│ 1. Load decision│   │ (batch up to 100)│   │ (batch up to 300)  │
│ 2. Redis EVAL:  │   │                  │   │                    │
│    - Confirm    │   │ On success:      │   │ Transaction:       │
│      frequency  │   │   UPDATE outbox  │   │ 1. Load decisions  │
│    - Confirm    │   │   SET PUBLISHED  │   │    (batch IN)      │
│      budget     │   │                  │   │ 2. Insert events   │
│                 │   │ On failure:      │   │    (IGNORE dupes)  │
│ On success:     │   │   Retry 8x       │   │ 3. Update metrics  │
│   UPDATE SETTLED│   │   Then DLQ       │   │    (UPSERT)        │
│                 │   │                  │   │ 4. Commit offsets  │
│ On missing proof│   │                  │   │                    │
│   UPDATE        │   │                  │   │                    │
│   RECONCILE     │   │                  │   │                    │
└─────────────────┘   └──────────────────┘   └────────────────────┘

Timeline (async):
- Client receives 202: 10-30ms (DB write only)
- Settlement: 100-500ms (depends on worker poll)
- Publication: 100-500ms (depends on relay poll)
- Consumption: 100-1000ms (depends on consumer lag)
- End-to-end: 500-2000ms typical
```

## 4. Campaign Publish Flow

Publishing makes a campaign's editable draft immutable and activates it.

```
┌────────┐
│ Admin  │
└───┬────┘
    │
    │ POST /v1/campaigns/{id}/publish
    │ Authorization: Bearer <admin-jwt>
    │
    ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Authentication & Authorization                 │
└───────────────────────────────┬─────────────────────────────────┘
    │
    │ 1. Verify JWT signature
    │ 2. Check role = "admin"
    ├─[UNAUTHORIZED]─→ HTTP 401/403
    │
    ▼ [AUTHORIZED]
┌─────────────────────────────────────────────────────────────────┐
│                      Campaign Service                            │
└───────────────────────────────┬─────────────────────────────────┘
    │
    │ 1. Load current campaign (MySQL)
    ├─[NOT FOUND]─→ HTTP 404
    │
    │ 2. Validate state = "draft"
    ├─[INVALID STATE]─→ HTTP 409 Conflict
    │
    │ 3. Validate business rules:
    │    - Has creatives
    │    - Budget > 0
    │    - Valid targeting
    ├─[INVALID]─→ HTTP 400 Bad Request
    │
    ▼ [VALID]
┌─────────────────────────────────────────────────────────────────┐
│                    MySQL Transaction BEGIN                       │
│                                                                  │
│  1. UPDATE campaigns                                            │
│     SET status = 'published',                                   │
│         published_version = version + 1,                        │
│         version = version + 1                                   │
│     WHERE id = ? AND version = current_version                  │
│                                                                  │
│  2. Check affected_rows = 1                                     │
│     [IF 0] → Optimistic lock failure → ROLLBACK → HTTP 409     │
│                                                                  │
│  3. INSERT INTO audit_logs                                      │
│     (action: "campaign.published", user, timestamp)             │
│                                                                  │
│  COMMIT                                                          │
└────────────────────────────────┬────────────────────────────────┘
                                 │
                                 ▼
                      ┌──────────────────────┐
                      │  HTTP 200 OK         │
                      │  + Updated Campaign  │
                      └──────────┬───────────┘
                                 │
                                 ▼
                           ┌──────────┐
                           │  Admin   │
                           └──────────┘

Side effects:
- Candidate cache: Stale for up to 5 seconds (TTL-based invalidation)
- Active decisions: Unaffected (use published_version at decision time)

Timeline: 15-30ms
```

## 5. Profile Cache-Aside Flow

Profiles are read-heavy, so we use cache-aside with Redis.

```
┌────────────────┐
│ Decision       │
│ Service        │
└───────┬────────┘
        │
        │ FindProfile(userId)
        │
        ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Profile Cache Adapter                          │
└───────────────────────────────┬─────────────────────────────────┘
        │
        │ 1. Check Redis: GET adflow:profile:{userId}
        │
        ├─[HIT]──→ Parse JSON → Return Profile
        │           (metrics: cache hit)
        │
        └─[MISS]
                │
                ▼
        ┌───────────────────────┐
        │ Load from MySQL       │
        │ (source of truth)     │
        └────────┬──────────────┘
                 │
                 ├─[FOUND]
                 │    │
                 │    │ Store in Redis:
                 │    │   SET adflow:profile:{userId} <json>
                 │    │   EXPIRE 300 (5 minutes)
                 │    │
                 │    └─→ Return Profile
                 │         (metrics: cache miss)
                 │
                 └─[NOT FOUND]
                      │
                      │ Negative cache:
                      │   SET adflow:profile:{userId} "null"
                      │   EXPIRE 5 (5 seconds)
                      │
                      └─→ Return ErrProfileNotFound

On Profile Update/Delete:
    │
    │ PutProfile / DeleteProfile
    │
    ▼
┌────────────────────────────────┐
│ 1. Write to MySQL (source)     │
│                                 │
│ 2. Invalidate Redis:           │
│    DEL adflow:profile:{userId} │
└────────────────────────────────┘

Timeline:
- Cache hit: 1-3ms
- Cache miss (MySQL): 10-20ms
- Write + invalidate: 15-30ms
```

## 6. Rate Limiting Flow (Redis Sliding Window)

Shared rate limiting across multiple API instances.

```
HTTP Request
     │
     ▼
┌─────────────────────────────────────────────────────────────────┐
│                   Redis Sliding Window Limiter                   │
└───────────────────────────────┬─────────────────────────────────┘
     │
     │ EVAL (Lua script on Redis server):
     │
     │   key = "adflow:admission:decision"
     │   now = server_time (avoid clock skew)
     │   window_start = now - window_duration
     │
     │   1. ZREMRANGEBYSCORE key -inf window_start
     │      (remove expired entries)
     │
     │   2. count = ZCARD key
     │      (count requests in window)
     │
     │   3. IF count >= limit THEN
     │        return {allowed: false, remaining: 0}
     │      END
     │
     │   4. ZADD key now now
     │      (add current request)
     │
     │   5. EXPIRE key (window_duration * 2)
     │      (cleanup if no traffic)
     │
     │   6. return {allowed: true, remaining: limit - count - 1}
     │
     ├─[ALLOWED]──→ Continue to service
     │
     └─[REJECTED]─→ HTTP 429 Too Many Requests
                     Retry-After: <window_duration>

Timeline: 2-5ms (Redis network + Lua execution)
```

## 7. Error Paths

### Decision Timeout
```
Decision Service
     │
     │ Processing...
     │
     ▼ (After configured timeout, e.g., 500ms)
┌──────────────────────────────┐
│ Context deadline exceeded    │
│                              │
│ Cleanup:                     │
│ 1. Release frequency token   │
│    (Redis DEL)               │
│ 2. Release budget token      │
│    (Redis DEL)               │
│ 3. Release decision lock     │
│    (MySQL UPDATE)            │
└────────────┬─────────────────┘
             │
             ▼
     HTTP 504 Gateway Timeout
```

### Database Connection Pool Exhaustion
```
HTTP Request
     │
     ▼
Query MySQL
     │
     ├─[Pool has idle connection]─→ Execute immediately
     │
     └─[Pool exhausted]
           │
           │ Wait for available connection
           │ (up to queue timeout, e.g., 5ms)
           │
           ├─[Connection available]─→ Execute
           │
           └─[Timeout]
                 │
                 ▼
         HTTP 503 Service Unavailable

Metrics:
- adflow_db_pool_wait_count_total (incremented)
- adflow_db_pool_wait_duration_seconds_total (added)
```

### Kafka Consumer Failure
```
Kafka Consumer
     │
     │ Fetch batch
     │
     ▼
Process events
     │
     ├─[Success]─→ Commit offset → Continue
     │
     └─[Error in processing]
           │
           │ DO NOT commit offset
           │
           ▼
     Rewind partition to failed offset
     │
     │ Retry from failed message
     │
     └─(After multiple retries, same message still fails)
           │
           │ Log error
           │ Skip message
           │ Commit offset
           │
           └─(Manual investigation required)
```

---

## Performance Characteristics

| Operation | P50 | P95 | P99 | Notes |
|-----------|-----|-----|-----|-------|
| Decision (matched) | 60ms | 100ms | 150ms | Hot path, Redis cached |
| Decision (no-ad) | 30ms | 50ms | 80ms | Faster, no reservations |
| Event ingestion | 15ms | 30ms | 50ms | Async, DB write only |
| Campaign CRUD | 10ms | 20ms | 40ms | Simple DB operations |
| Profile GET (cached) | 2ms | 5ms | 10ms | Redis hit |
| Profile GET (uncached) | 12ms | 20ms | 35ms | MySQL fallback |

**Environment:** Single instance, cross-host MySQL/Redis, Windows WSL

---

**Last updated:** 2026-09-08
