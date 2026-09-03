# ADR-0004: Cache immutable decision candidates by slot

- Status: Accepted
- Date: 2026-09-03

## Context

Every advertising decision previously loaded active campaign versions and creative IDs from MySQL. This data is shared by all requests for a slot and changes through relatively infrequent administration operations. Profiles, frequency counts, budgets, reservations, and decision idempotency are request-specific and must remain live.

A real 20-campaign load test showed that repeated candidate reads consumed database connections and amplified cross-host latency. Concurrent requests also refreshed the same slot independently when no cache was present.

## Decision

Wrap the repository-backed candidate provider with a per-process immutable snapshot keyed by slot.

- Default TTL: five seconds, configurable with `ADFLOW_CANDIDATE_CACHE_TTL`.
- Concurrency: one goroutine owns a refresh for a slot; concurrent misses wait on the same result.
- Isolation: every caller receives a deep copy of creative IDs and targeting conditions.
- Memory bound: at most 1,024 slot snapshots are retained; the earliest-expiring entry is evicted when the bound is reached.
- Scope: only campaign/version/creative candidate configuration is cached.
- Observability: Prometheus records hit, miss, shared-wait, error, and refresh duration.

The source provider remains authoritative and continues filtering by active status, slot, delivery period, and active creatives. No distributed invalidation channel is introduced at this stage.

## Consequences

Benefits:

- Hot decisions avoid repeated campaign and creative SQL queries.
- A cold or expired slot produces one refresh instead of a database stampede.
- Snapshots cannot be mutated through slices returned to decision evaluation.
- Cache effectiveness and refresh spikes are measurable.

Costs:

- An administration change may take up to five seconds to reach every API process.
- Every API replica has its own cache and refresh schedule.
- A refresh is synchronous for the first request and its coalesced waiters.

The bounded propagation delay is acceptable for campaign configuration. Budget, frequency, profile, reservation, and idempotency correctness do not depend on this cache.

## Verification

Thirty-two concurrent cold reads produced one source call in the race-safe unit test. Against real MySQL/Redis/Kafka infrastructure, the cache reduced the 100 iteration/s decision error rate from 35.41% to approximately 2%–3% and established a zero-error 80 iteration/s full-path baseline with no Outbox backlog or Kafka consumer lag.
