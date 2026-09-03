# AdFlow Roadmap

## Current baseline

The modular monolith already covers the complete MVP path from campaign configuration to decision, event ingestion, and delivery metrics. The codebase includes Redis Lua reservations, optimistic campaign revisions, Kafka adapters, a transactional outbox, consumer idempotency, dead-letter publication, Prometheus metrics, and an approval-gated Agent rule assistant.

## P0 — Reproducible engineering

- Keep the backend and frontend in one repository.
- Run Go formatting, vet, race-enabled tests, and coverage in CI.
- Run frontend formatting, lint, and production builds in CI.
- Keep the local in-memory mode as the fast development path.

## P1 — Concurrency and persistence proof

- The admission baseline now includes a local token bucket, optional Redis ZSET sliding window, bounded per-process execution concurrency, queue timeout, processing deadline, cancellation-safe reservation cleanup, and overload metrics.
- The opt-in real MySQL, Redis, and Kafka suite now verifies optimistic revisions, concurrent Outbox lease ownership, atomic admission, duplicate publication, and consumer restart/replay.
- A repeatable migration command applies ordered SQL files under a MySQL advisory lock and verifies stored checksums.
- Kafka acknowledgment-before-Outbox-mark and side-effect-before-offset-commit failure boundaries are covered with persistent `eventId` idempotency assertions.
- The sustained k6 scenario now supports configurable campaign/profile cardinality, warm-up, fixed arrival rate, decision-only isolation, and the full decision-to-impression path.
- The first real-adapter run is recorded: it exposed and removed candidate-loading N+1 reads, established a stable 20 iteration/s full-path baseline, and showed that 100 iteration/s still exceeds the local cross-host database path.
- Kafka consumption now runs partitions concurrently, preserves in-partition order, batches contiguous offset commits, and rewinds a failed partition for retry.
- Consumer persistence and Outbox publication now use idempotent batches; the final 50 iteration/s run completed with zero errors, zero Outbox backlog, and zero Kafka lag.
- A five-second immutable candidate snapshot with deep-copy reads, miss coalescing, and Prometheus metrics raises the stable full-path baseline from 50 to 80 iteration/s.
- Redis/MySQL Profile Cache-Aside with miss coalescing, negative caching, write-through updates, and MySQL fallback raises the stable full-path baseline from 80 to 100 iteration/s, including a zero-error cold-fill run.
- Request-specific Decision and Outbox writes are the remaining measured hot-path database work.
- Add longer broker-outage, partition-reassignment, lease-expiry, and dead-letter replay chaos scenarios.
- Record repeatable P50, P95, P99, throughput, error rate, database pool usage, Redis latency, outbox depth, and Kafka lag.

## P2 — Database depth

- Review query plans with `EXPLAIN ANALYZE` against production-shaped data.
- Document transaction boundaries, isolation assumptions, lock behavior, and idempotency constraints.
- Add index regression checks for campaign lookup, outbox claiming, event deduplication, and metric aggregation.
- Define data retention and archival policies for decisions, receipts, processed events, and dead letters.

## P3 — Administration security (baseline implemented)

- Extend the implemented JWT and hierarchical RBAC baseline with refresh-token rotation or external identity when production integration requires it.
- Move from action-level append-only audit writes to transaction-coupled audit/outbox records for operations that require strict compliance guarantees.
- Add the administration UI login flow and audit-log workspace.
- Add request rate limits and administrative action protection where appropriate.

## P4 — Agent and operations

- The Agent baseline now includes Responses and Chat Completions adapters, strict structured decoding, safety limits, bounded retries, circuit breaking, Mock fallback, usage telemetry, and an opt-in evaluation set.
- Run and record the live evaluation set against the selected production model before enabling it outside development.
- Add operator views and APIs for outbox backlog, Kafka lag, dead-letter inspection, and controlled replay.
- Add dashboards and alerts for decision latency, error rate, reservation failures, outbox age, and consumer lag.

## P5 — Frontend follow-up

- Add campaign detail and immutable-version comparison views.
- Add delivery trends and operational Kafka/Outbox panels using real metrics.
- Improve narrow-screen data-table workflows and add component-level tests.
