# ADR-0005: Use Redis/MySQL Cache-Aside for decision profiles

- Status: Accepted
- Date: 2026-09-03

## Context

Every decision requires a user profile. MySQL provides durable profile storage, but a real 100 iteration/s test showed that one profile query per decision consumed connection-pool capacity and pushed the decision path beyond its latency/error threshold. Unlike campaign candidates, profiles are keyed by user, have much higher cardinality, and are shared across API replicas.

## Decision

Add an explicit `mysql-redis` ProfileStore adapter.

- MySQL remains the source of truth.
- Positive profiles use a five-minute Redis TTL.
- Missing users use a five-second negative TTL to limit penetration traffic.
- Concurrent misses for one user are coalesced per API process.
- Profile fills and writes share a striped per-user lock so an older in-flight fill cannot overwrite a newer write in the same process.
- Redis reads fail open to MySQL so a cache outage degrades latency rather than availability.
- Cache operations have an independent 10 ms deadline so Redis failure does not consume the decision request's full processing budget before MySQL fallback.
- Profile PUT writes MySQL first and then replaces the Redis value.
- Cached values and returned profiles are copied so maps are not shared with callers.
- Prometheus records hits, misses, fills, negative hits/fills, shared waits, and cache/source errors.

## Consistency

The normal write path updates the source of truth before Redis. If the Redis update fails after MySQL commits, the PUT reports an error and can be retried safely because profile replacement is idempotent. An older cache entry may remain until its bounded TTL if Redis is unavailable during that write. Direct database writes outside the application bypass cache coordination and are not supported as an immediate-consistency path.

## Consequences

Benefits:

- Repeated decisions avoid MySQL profile reads.
- Redis is shared across replicas, unlike a per-process profile map.
- Negative caching protects MySQL from repeated unknown-user lookups.
- A Redis outage does not automatically become a decision outage.

Costs:

- Profile data is eventually consistent if a write-through update fails.
- Cold users still require a MySQL lookup.
- Per-process miss coalescing does not provide a distributed lock across replicas.

## Verification

Unit tests cover positive fill/hit, defensive copies, negative-cache expiry, write-through replacement, Redis-outage fallback, and 32 concurrent misses collapsing into one source lookup.

Against real MySQL, Redis, and Kafka, an empty profile cache completed 2,524 full-path iterations at a 100 iteration/s target with 500 MySQL fills, 2,024 Redis hits, zero errors, zero Outbox backlog, and zero Kafka lag. The following hot-cache run also completed 2,524 iterations with zero errors and reduced decision P95 from 157.90 ms to 100.10 ms.
