# AdFlow Roadmap

## Current baseline

The modular monolith already covers the complete MVP path from campaign configuration to decision, event ingestion, and delivery metrics. The codebase includes Redis Lua reservations, optimistic campaign revisions, Kafka adapters, a transactional outbox, consumer idempotency, dead-letter publication, Prometheus metrics, and an approval-gated Agent rule assistant.

## P0 — Reproducible engineering

- Keep the backend and frontend in one repository.
- Run Go formatting, vet, race-enabled tests, and coverage in CI.
- Run frontend formatting, lint, and production builds in CI.
- Keep the local in-memory mode as the fast development path.

## P1 — Concurrency and persistence proof

- The in-process baseline now includes token-bucket admission, bounded execution concurrency, queue timeout, processing deadline, cancellation-safe reservation cleanup, and overload metrics.
- Add a real MySQL, Redis, and Kafka integration environment when container work resumes.
- Apply every migration through a repeatable migration command rather than mounting only the first SQL file.
- Test process restart after Kafka acknowledges an event but before the outbox row is marked published.
- Test duplicate Kafka delivery, consumer restart, lease expiry, concurrent outbox relays, and Redis reservation release.
- Run sustained load tests with representative campaign, creative, and profile cardinality.
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

- Add a real model-provider adapter with timeout, retry, structured output validation, and usage telemetry.
- Build a small evaluation set for targeting-rule generation.
- Add operator views and APIs for outbox backlog, Kafka lag, dead-letter inspection, and controlled replay.
- Add dashboards and alerts for decision latency, error rate, reservation failures, outbox age, and consumer lag.

## P5 — Frontend follow-up

- Add campaign detail and immutable-version comparison views.
- Add delivery trends and operational Kafka/Outbox panels using real metrics.
- Improve narrow-screen data-table workflows and add component-level tests.
