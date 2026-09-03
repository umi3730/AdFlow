# ADR-0002: Redis sliding-window admission across API replicas

- Status: Accepted
- Date: 2026-09-03

## Context

The in-process token bucket protects one API instance, but its effective global limit grows when replicas are added. AdFlow needs an optional shared request-rate boundary while retaining a dependency-free local mode. Rate admission and execution concurrency solve different problems: a shared rate limit controls aggregate arrivals, while each process still needs a local cap that protects its CPU, database pool, Redis connections, and downstream calls.

## Decision

Keep the rate limiter behind the decision application port and provide two adapters:

1. `memory`: synchronized token bucket with burst capacity for local development and single-process deployments.
2. `redis`: exact sliding-window log stored in a sorted set for multi-replica admission.

The Redis adapter executes one Lua script per request. The script:

- reads Redis server time;
- removes members at or before the window cutoff;
- counts the remaining members;
- rejects when the configured limit is reached;
- otherwise inserts a process-unique request member; and
- refreshes a TTL of two window lengths so inactive keys are reclaimed.

The script is atomic because Redis executes Lua without interleaving other commands. Redis server time avoids admission differences caused by clock skew between API hosts. A Redis error fails closed with HTTP 503. An explicit over-limit decision returns HTTP 429.

The local semaphore, queue timeout, and execution deadline remain active with either rate-limiter adapter.

## Consequences

Benefits:

- all API replicas share one exact rolling request window;
- window cleanup and admission cannot race;
- inactive limiter state expires automatically;
- local development does not require Redis.

Costs and limits:

- the global ZSET is a hot key and each request performs `O(log N)` sorted-set work;
- exact sliding logs use more memory than fixed-window counters or GCRA;
- Redis becomes part of the synchronous decision path and must be highly available;
- a single global key is not a multi-region coordination strategy.

If throughput grows beyond the exact-window design, benchmark a sharded or gateway-level GCRA/token-bucket implementation before changing semantics.
