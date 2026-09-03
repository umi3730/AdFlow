# ADR-0001: Use Kafka as the advertising event backbone

- Status: Accepted
- Date: 2026-09-02

## Context

AdFlow receives impression, click, and conversion events after an advertising decision. These events must be retained long enough for replay, processed by independently scalable consumers, and ordered for a single decision so an impression precedes its click and conversion.

Redis remains appropriate for low-latency frequency and budget reservations. Using the same Redis instance as both the online state store and the durable event backbone would couple two workloads with different retention and scaling requirements.

## Decision

Use Kafka for the advertising event backbone and franz-go as the Go client.

- Topic: `adflow.ad-events.v1`
- Record key: `requestId`
- Envelope: JSON with an explicit `schemaVersion`
- Consumer group: `adflow-metrics-v1`
- Delivery semantics: at least once
- Offset policy: manual commit after successful processing
- Idempotency key: `eventId`

Using `requestId` as the record key routes all events for one advertising decision to the same partition. Kafka preserves order within that partition, while different decisions can be processed in parallel.

## Consequences

Benefits:

- Event retention and replay are independent from online Redis TTLs.
- Consumer groups allow metrics, billing, attribution, and audit consumers to scale independently.
- Partition ordering supports the impression-before-click/conversion rule.
- Schema versioning provides an explicit compatibility boundary.

Costs:

- Kafka adds operational complexity, broker health, partitions, lag monitoring, and rebalance behavior.
- At-least-once delivery requires every consumer side effect to be idempotent.
- A producer acknowledgment alone is not enough to atomically persist business data and publish Kafka.

## Current implementation boundary

The repository now includes:

1. A configurable Kafka producer and manual-commit consumer group.
2. A versioned event codec and synchronous local fallback.
3. An `event_receipts` plus `event_outbox` transaction at ingestion.
4. Lease-based outbox batch claiming that supports multiple relay instances on MySQL 5.7.
5. Kafka acknowledgment before an outbox row is marked published.
6. Exponential publish retry after a failed acknowledgment.
7. A unique `processed_events.event_id` consumer barrier and transactional metric aggregation.
8. A dead-letter topic after eight failed relay attempts.
9. Prometheus gauges for Outbox states and per-partition Consumer Lag.
10. Durable decision records used to validate delayed events after process restarts.

The implementation claims at-least-once delivery with idempotent processing, not exactly once. A crash after Kafka accepts a record but before the relay marks the outbox row published can produce a duplicate; the consumer must safely absorb it.

## Verification status

The repository now has opt-in real-infrastructure tests for the two ambiguous at-least-once failure boundaries:

1. Kafka acknowledges a record, the relay stops before marking its Outbox row, and the next relay publishes the record again. Both records are consumed while the unique `eventId` barrier produces one metric side effect.
2. A consumer commits the MySQL side effect and then stops before committing its Kafka offset. A restarted consumer replays the record, observes the existing `eventId`, and commits without duplicating the metric.

The same suite also verifies MySQL optimistic concurrency, lease competition between two Outbox relays, and Redis Lua atomicity against real services. This establishes the local correctness baseline; sustained broker outage, partition reassignment, retention, and production-scale load remain operational proof obligations.
