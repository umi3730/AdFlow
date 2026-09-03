# Real infrastructure verification

## Scope

The integration suite runs the Go application code natively against external MySQL, Redis, and Kafka services. It is intentionally opt-in so `go test ./...` stays fast and does not require infrastructure.

The baseline was verified on 2026-09-03 with Go on Windows, MySQL 8.0.46 in WSL, Redis 8.10.1, and Apache Kafka 4.3.1. The test does not require or produce an AdFlow application container image.

## Preparation

1. Create an empty MySQL database and a least-privilege test user. Do not run a long-lived AdFlow API or another Outbox relay against this database while the suite is running; the fault-injection tests intentionally control relay ownership and timing.
2. Set `ADFLOW_MYSQL_DSN` and run `go run ./cmd/migrate -dir migrations` twice. The first run applies the files; the second must report an empty applied list.
3. Create the Kafka topics configured by `ADFLOW_IT_KAFKA_TOPIC`, `ADFLOW_IT_KAFKA_DEAD_LETTER_TOPIC`, and `ADFLOW_IT_KAFKA_RESTART_TOPIC`.
4. Export the variables shown in `.env.integration.example`, replacing the example credentials.
5. Run `go test -tags=integration -v ./tests/integration`.

## Failure timelines under test

### Producer acknowledgment before Outbox completion

1. Event ingestion transaction writes the receipt and Outbox row.
2. Kafka accepts the event.
3. The test simulates the relay stopping before `published_at` is written.
4. A replacement relay claims the same Outbox row and publishes it again.
5. The consumer processes both deliveries, while the MySQL `processed_events.event_id` barrier allows exactly one metric increment.

This demonstrates at-least-once publication plus idempotent consumption. It does not claim exactly-once Kafka delivery.

### Side effect before offset commit

1. The first consumer writes the event side effect to MySQL.
2. An injected failure stops processing before the Kafka offset is committed.
3. A second consumer with the same group receives the event again.
4. The persistent `eventId` barrier reports the event as already processed.
5. The second consumer safely commits the offset; a third consumer observes no further delivery.

## Defects exposed by the real services

- MySQL `DATETIME(3)` truncated an Outbox lease timestamp to milliseconds while the claim lookup retained nanoseconds. Normalizing both claim timestamps to millisecond precision fixed the missed-row lookup.
- Redis reservation metadata used a relative TTL while its daily amount keys used application-computed absolute expiry timestamps. Cross-host clock skew could expire the amount keys before confirmation. All daily reservation state now uses relative TTLs, with a regression test that gives Redis and the Go process a 72-hour clock offset.

## Remaining evidence

The suite proves local correctness at the principal duplicate-delivery boundaries. It does not replace sustained throughput tests, multi-broker partition reassignment, prolonged outage recovery, retention validation, or production monitoring and alert drills.
