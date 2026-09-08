# AdFlow Roadmap

## Current baseline

The modular monolith already covers the complete MVP path from campaign configuration to decision, event ingestion, and delivery metrics. The codebase includes Redis Lua reservations, optimistic campaign revisions, Kafka adapters, a transactional outbox, consumer idempotency, dead-letter publication, Prometheus metrics, and an approval-gated Agent rule assistant.

## P0 — Reproducible engineering

- Keep the backend and frontend in one repository.
- Run Go formatting, vet, race-enabled tests, and coverage in CI.
- Run frontend formatting, lint, and production builds in CI.
- Keep the local in-memory mode as the fast development path.

## P1 — Concurrency and persistence proof

- Async backlog protection now samples capped active settlement/Outbox queues with high/low watermarks, blocks only new decisions before reservations, and fails closed on unavailable/stale observations. Real Kafka 100 attempts/s x 120s admitted 6,681 full flows and explicitly rejected 5,320; accepted events reconciled with no unexpected errors or RECONCILE, and 45/s recovery had no rejections. This is overload protection, not 100 full flows/s capacity. See `async-backpressure-20260908.md`.

- Post-fix longer Kafka capacity confirmation: 45 business rounds/s (~180 HTTP QPS) passed 3 x 120s with 16,202 complete flows / 48,606 events, no errors or reconciliation, and flat queue trends. 50 rounds/s met finite-window SLO but its third queue grew; 55 exceeded latency; 60/65 exposed overload-driven reservation expiry. See `kafka-capacity-followup-20260908.md`. Next: backlog-aware admission and measured settlement/audit database cost; no production-max claim.

- Fixed the deadlock/lease recovery blocker: owner-fenced unfinished-claim release, shorter bounded claims, READ COMMITTED and rollback-confirmed retries for settlement/ingress/publication transactions. Real deadlock and ownership-recovery tests pass; final 3 x 60s Kafka runs at 50 business rounds/s processed 27,009 measured events with no errors or reconciliation. See `settlement-recovery-fix-20260908.md`; older failure findings below remain historical evidence.

- Kafka full-business capacity testing found a recovery blocker: 60 rounds/s passed three finite 60s windows but queues grew; a separate 50 rounds/s confirmation hit an Outbox index deadlock, followed ~30s later by three settlements entering RECONCILE. No reliable full-chain stable capacity claim is made. See `kafka-capacity-20260908.md` for deadlock evidence and the unprocessed-batch lease/30s reservation interaction. Production fix remains pending.

- Matched decision capacity verified at an offered 450 QPS in three 60s confirmations: 81,003 requests, 81,001 valid auction results, two HTTP 409s, no No-Ad or dropped iterations, P95 100-103ms under a predeclared <300ms / <0.1% error SLO. Full ramp/failure/long-run evidence and reservation checks are in `decision-capacity-20260908.md`; this excludes impression/Kafka processing and is not absolute production capacity.

- Profile GET arrival-rate capacity exploration completed: 1000 QPS passed three 60s confirmations under P95<100ms, error<0.1%, zero dropped iterations; 1500 QPS failed a longer repeat and 2000 QPS failed its probe. This is the highest verified local tier on a 500-QPS grid, not an absolute server maximum. Resource/goroutine samples and all failed runs are retained in `profile-capacity-20260908.md`.

- Full-chain index visibility comparison completed on a 1M-row synthetic Outbox background: 1,440 measured HTTP decision-to-impression/click/conversion flows, 1,476 settlements including warmup, and 4,428 processed events all reconciled. Under a static 5% stranded SETTLING fixture, committed-observation P95 changed from 17.82s to 1.29s; healthy history remained about 1.3s. This is a 10 rounds/s bounded workload, not maximum capacity. See `full-chain-experiment-20260908.md`.

- Fixed the local 100-concurrency simulator timeout regression with driver parameter interpolation, typed JSON parameters, nonblocking queue claims and an Outbox request index. The original browser configuration completed 3000 rounds with zero failures/timeouts; measurements and workload boundaries are in `simulation-timeout-fix-20260907.md`.

- Kafka settlement now has a durable queue, atomic Redis budget/frequency confirmation with retry receipts, fenced worker leases, publication dependencies, and consumer proof checks. Local fault-injection regressions pass; real infrastructure acceptance remains pending. See `outbox-settlement-fix-20260907.md`; existing capacity numbers below predate this change.

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

- JMeter profile GET A/B completed (1,000 saved users, 20 threads, 3 x 15s per adapter): both pooled P95 values were 13ms and throughput about 2k requests/s, so no material latency gain was established. Hot Redis eliminated user-table reads for all 92,784 samples including JMeter warmup. Raw JTL and cross-checked counters are in `jmeter-profile-experiment-20260908.md`.
- Completed an isolated Outbox request-index visibility A/B at 100k and 1M synthetic rows: 6 paired rounds, 480 measured statements, actual MySQL statement counters, plans and independent percentile verification. At 1M rows, the request UPDATE server P95 fell from 164.52ms to 1.09ms; this excludes transaction commit and is not an API-capacity claim. See `sql-index-experiment-20260908.md` and `tests/bench/outbox-index/`.
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

- Read-only request inspection is available from single/batch test results and the operations workspace. It joins retained decision snapshots, settlement, publication and metric evidence without replaying requests; see `request-trace-20260907.md`.

- The Agent baseline now includes Responses and Chat Completions adapters, strict structured decoding, safety limits, bounded retries, circuit breaking, Mock fallback, usage telemetry, and an opt-in evaluation set.
- The DeepSeek V4 Flash live evaluation is recorded: default-thinking and taxonomy failures drove compatibility/prompt fixes, after which all four safety/semantic cases and one HTTP end-to-end request passed.
- Outbox backlog, Kafka lag, dead-letter inspection, and admin-only replay are exposed through operator APIs and the responsive administration console.
- Add dashboards and alerts for decision latency, error rate, reservation failures, outbox age, and consumer lag.

## P5 — Frontend follow-up

- Saved-profile catalog, full-attribute editing, user selection and side-effect-free current-targeting explanations are implemented; see `profile-workspace.md`.
- Implemented: bounded browser user-pool simulation with request-scoped temporary profiles (no catalog writes), optional saved-profile mode, fixed concurrency (up to 100 rounds in flight), bounded duration/round count, cancellation, decision-to-impression traffic and report export. See `user-pool-simulation.md`; this is distinct from reproducible k6 capacity tests.
- Add campaign detail and immutable-version comparison views.
- The responsive Kafka/Outbox operations workspace is implemented; add longer-window delivery and lag trends after a metrics time-series backend is selected.
- Improve narrow-screen data-table workflows and add component-level tests.

- Built-in editable/deletable demo fixture and idempotent startup markers are implemented; see `demo-data.md`. Additional code-review findings are recorded in `feature-review-20260907.md`.
