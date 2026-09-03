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

The next optimization target is the per-event MySQL transaction in the consumer. Candidate snapshots/caching and batched metric persistence should be evaluated before raising the supported arrival-rate baseline.
