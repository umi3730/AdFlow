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
