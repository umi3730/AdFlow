# AdFlow Performance Baseline

## Baseline 001 — local in-memory smoke load

- Date: 2026-09-02
- Machine: Windows amd64, Intel Core i7-8700 @ 3.20 GHz
- Go: 1.27.0
- k6: 2.2.0
- Campaign repository: memory
- Reservation adapter: memory
- Load: 5 virtual users for 5 seconds
- Work per iteration: one profile PUT followed by one decision POST

### Results

| Metric | Result |
| --- | ---: |
| Completed decision iterations | 8,931 |
| Decision iterations per second | 1,775.16/s |
| Total HTTP requests per second | 3,550.93/s |
| Decision latency average | 1.72 ms |
| Decision latency P95 | 4 ms |
| Decision latency P99 | 6 ms |
| Decision latency maximum | 39 ms |
| Decision error rate | 0.00% |

### Interpretation

This is a short local smoke load, not a production capacity claim. It excludes MySQL and Redis network costs, uses one process, contains a single active campaign, and creates a unique in-memory profile for each iteration. The result proves that the load script, metrics, targeting path, and concurrency controls can run together without errors under a small concurrent load.

Do not copy these numbers into a resume as production QPS. A resume baseline should use a longer warm-up and steady-state period, Redis reservations, representative campaign/profile cardinality, fixed machine details, and repeated runs.

## Microbenchmark

The pure targeting evaluator benchmark completed at approximately 123.8 ns/op with 0 B/op and 0 allocations/op on the same machine. This measures only in-process rule evaluation and must not be compared directly with HTTP decision latency.

The admission-controller fast-path benchmark completed at approximately 469.6 ns/op with 272 B/op and 4 allocations/op. It includes the synchronized token bucket, uncontended semaphore admission, and request deadline context. This is a local microbenchmark rather than a production capacity result.

## Baseline 002 — admission-control spike

- Date: 2026-09-03
- Machine: Windows amd64, Intel Core i7-8700 @ 3.20 GHz
- Storage adapters: in-memory
- Admission settings: 100 requests/s, burst 20, max in-flight 2, queue timeout 1 ms, execution timeout 100 ms
- Load: ramping arrival rate from 20 to 300 iterations/s for 23 seconds

| Metric | Result |
| --- | ---: |
| Total decision iterations | 4,379 |
| Admitted decisions | 1,826 |
| Explicitly rate-limited decisions | 2,553 |
| Admission rejection rate | 58.30% |
| Unexpected responses | 0 |
| HTTP 500 responses | 0 |
| Decision-duration P99 | 2 ms |
| In-flight decisions after test | 0 |
| Execution timeouts | 0 |

This deliberately constrained run demonstrates stable backpressure rather than maximum throughput. HTTP 429 is an expected result. The separate concurrency test verifies that active decision execution never exceeds the configured semaphore capacity, while the spike run verifies that excess arrival rate does not become an internal server error.

## Baseline 003 — real MySQL, Redis, and Kafka full path

- Date: 2026-09-03
- Machine: Windows amd64, Intel Core i7-8700 @ 3.20 GHz
- Process placement: native Windows API and k6; MySQL 8.0.46, Redis 8.10.1, and Kafka 4.3.1 in WSL/Docker
- Storage adapters: MySQL campaigns, profiles, decisions, Outbox, and metrics; Redis admission and reservations
- Event transport: Kafka with three partitions and one consumer process
- Cardinality: 20 active campaigns, 20 creatives, and 500 reusable profiles
- Work per iteration: one decision followed by one impression event

### Failure-driven optimization

The first 100 iteration/s run exposed a candidate-loading N+1 query. Every decision loaded the campaign list and then queried creatives once for each of the 20 campaigns. A single idle request exceeded its 500 ms deadline, and all 3,799 load iterations returned HTTP 504 at approximately 502 ms.

Candidate loading was changed to filter active campaigns by `slot_id` in SQL and fetch all active creative IDs for the candidate campaign set in one query. This reduced candidate reads from 21 SQL queries to two. The same 100 iteration/s workload then completed 2,545 of 3,799 decisions and impressions, reducing the error rate from 100% to 33%. It still failed the latency and error thresholds, so 100 iteration/s is explicitly not a supported baseline for this cross-host environment.

### Stable 20 iteration/s run

The reusable dataset was retained with `RUN_ID=real-baseline-003` and `SEED_DATA=false`. The scenario ramped to 20 iterations/s, held for 20 seconds, and cooled down for 5 seconds.

| Metric | Result |
| --- | ---: |
| Completed full-path iterations | 505 |
| Decision and impression errors | 0.00% |
| Dropped iterations | 0 |
| Decision latency average | 188.91 ms |
| Decision latency P95 | 212.73 ms |
| Decision latency P99 | 237.97 ms |
| Impression-ingestion latency P95 | 199.17 ms |
| Full-path latency P95 | 412.39 ms |

These numbers include Windows-to-WSL network overhead and are a local engineering baseline, not a production capacity claim.

`EXPLAIN ANALYZE` after the run measured approximately 0.13 ms for the 20-row campaign/version query and 0.23 ms for the active-creative query. MySQL correctly preferred table scans at this tiny cardinality. The much larger HTTP latency is therefore dominated by cross-host round trips and transaction sequencing in this environment, not SQL execution time. Index decisions must be repeated against production-shaped row counts rather than inferred from this 20-row plan.

### Consumer backlog finding

The load run also exposed a second bottleneck: the consumer processed every Kafka partition serially and synchronously committed each record offset. The consumer now processes partitions concurrently while preserving record order within each partition, commits the highest contiguous successful offset once per fetch, and seeks a failed partition back to the failed record. The real restart/idempotency integration tests passed three consecutive runs after this change.

During backlog replay, the new consumer processed 273 events in the first measured 20-second interval (about 13.7 events/s) and subsequently reduced all three partition lags to zero. MySQL contained 3,050 processed events and 3,050 aggregate impressions after the drain.

These findings motivated the batched persistence experiment in Baseline 004. Candidate snapshots remain a separate online-path optimization.

## Baseline 004 — batched Kafka persistence pipeline

The consumer bottleneck from Baseline 003 was addressed with an atomic batch path:

- Decisions are fetched for the partition batch with one `IN` query.
- Existing impression dependencies are fetched once for click/conversion validation.
- Up to 300 decoded events enter one MySQL transaction.
- A generated `processing_batch_id` identifies exactly which `INSERT IGNORE` rows were newly created, so duplicate `eventId` values do not inflate metrics.
- Newly inserted events are aggregated by campaign and applied with one multi-row metric upsert.
- Kafka partitions run concurrently, remain ordered internally, and commit only their highest contiguous successful offsets.
- The Outbox relay publishes up to 100 events with one Kafka `ProduceSync` call and marks the successful batch with one MySQL update.

Migration `000006_event_processing_batches.up.sql` adds the nullable batch identifier and its lookup index.

### Final 50 iteration/s run

| Metric | Result |
| --- | ---: |
| Completed full-path iterations | 1,262 |
| Decision and impression errors | 0.00% |
| Dropped iterations | 0 |
| Decision latency average | 188.84 ms |
| Decision latency P95 | 235.69 ms |
| Decision latency P99 | 242.50 ms |
| Impression-ingestion latency P95 | 221.28 ms |
| Full-path latency P95 | 431 ms |
| Outbox rows not published after the run | 0 |
| Kafka consumer lag after the run | 0 on all three partitions |

### 100 iteration/s boundary run

The same final implementation completed 1,630 decision/impression paths, but 894 of 2,524 decision attempts reached the 500 ms deadline, for a 35.41% decision error rate. All 1,630 accepted impressions were published and consumed without residual Outbox backlog or Kafka lag.

This separates the remaining limitation from the asynchronous pipeline: the batched Outbox and consumer can drain every accepted event, while the synchronous decision path saturates near this local environment's database/network boundary. The supported local full-path baseline is therefore 50 iterations/s, not 100 iterations/s. The next experiment should introduce a short-lived immutable candidate snapshot and then repeat the same A/B workload.

## Baseline 005 — immutable candidate snapshot

Campaign/version and active-creative data changes much less frequently than decisions arrive. A per-process snapshot now caches the complete candidate list by slot, returns deep copies to callers, and coalesces concurrent cold/expired reads so one goroutine refreshes MySQL while the others wait. The cache never contains user profiles, budget, frequency, reservation, or decision-idempotency state.

At a one-second TTL, the 100 iteration/s test recorded 2,159 direct hits, 27 refresh misses, and 333 shared waits. Decision failures fell from 35.41% without the snapshot to 2.21%, and successful full paths increased from 1,630 to 2,468 of 2,524 attempts.

A five-second TTL reduced refresh pressure to six successful misses and 75 shared waits during the next 100 iteration/s run. That run still had 2.61% full-path errors, confirming that candidate refresh was no longer the main saturation point. Five seconds is the default because campaign changes can tolerate bounded eventual propagation while profile and budget data remain live.

### Stable 80 iteration/s run

| Metric | Result |
| --- | ---: |
| Completed full-path iterations | 2,019 |
| Decision and impression errors | 0.00% |
| Dropped iterations | 0 |
| Decision latency average | 101.87 ms |
| Decision latency P95 | 148.79 ms |
| Decision latency P99 | 161.66 ms |
| Impression-ingestion latency P95 | 220.09 ms |
| Full-path latency P95 | 346 ms |
| Outbox rows not published after the run | 0 |
| Kafka consumer lag after the run | 0 on all three partitions |

The supported local full-path baseline is therefore raised from 50 to 80 iterations/s. The unchanged 100 iteration/s threshold failure is retained as the next boundary: profile lookup, decision persistence, reservation calls, and Outbox ingestion still perform request-specific network operations and cannot be replaced by the shared candidate snapshot.

## Baseline 006 — Redis/MySQL profile Cache-Aside

The `mysql-redis` profile adapter keeps MySQL authoritative while caching positive profiles for five minutes and missing users for five seconds. Concurrent misses for one user are coalesced inside each API process. Profile writes update MySQL first and then replace the Redis representation; Redis read failures degrade to MySQL rather than failing the decision immediately.

The first run began with an empty Redis profile namespace and therefore included a real cold-fill phase:

| Metric | Cold-fill result at 100 iteration/s |
| --- | ---: |
| Completed full-path iterations | 2,524 |
| MySQL profile fills | 500 |
| Redis profile hits | 2,024 |
| Decision and impression errors | 0.00% |
| Decision latency average | 70.43 ms |
| Decision latency P95 | 157.90 ms |
| Decision latency P99 | 211.01 ms |
| Full-path latency P95 | 363.84 ms |

The immediately repeated hot-cache run also completed all 2,524 iterations without error:

| Metric | Hot-cache result at 100 iteration/s |
| --- | ---: |
| Decision latency average | 55.64 ms |
| Decision latency P95 | 100.10 ms |
| Decision latency P99 | 111.36 ms |
| Impression-ingestion latency P95 | 215.39 ms |
| Full-path latency P95 | 296 ms |
| Outbox rows not published after both runs | 0 |
| Kafka consumer lag after both runs | 0 on all three partitions |

Across both runs, Prometheus recorded exactly 500 source fills and 4,548 Redis hits. This raises the supported local full-path baseline from 80 to 100 iterations/s while retaining an explicit cold-cache measurement. The next performance work should target request-specific Decision/Outbox writes or a colocated Linux deployment rather than adding more shared configuration caches.
