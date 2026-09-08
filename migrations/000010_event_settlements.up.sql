CREATE TABLE event_settlements (
    request_id VARCHAR(128) NOT NULL PRIMARY KEY,
    event_id VARCHAR(128) NOT NULL UNIQUE,
    decision_snapshot JSON NOT NULL,
    accepted_at DATETIME(3) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    attempts INT UNSIGNED NOT NULL DEFAULT 0,
    next_attempt_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    locked_by VARCHAR(64) NULL,
    locked_until DATETIME(3) NULL,
    last_error VARCHAR(1024) NULL,
    settled_at DATETIME(3) NULL,
    KEY idx_settlement_claim (status, next_attempt_at, locked_until)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Stop old API/relay/consumer processes before migration. Legacy unprocessed
-- events lack settlement receipts: do not infer financial success from enqueue.
UPDATE event_outbox o LEFT JOIN processed_events p ON p.event_id = o.event_id
SET o.status = 'RECONCILE', o.locked_by = NULL, o.locked_until = NULL,
    o.last_error = 'Legacy event: settlement proof unavailable; reconcile before replay'
WHERE p.event_id IS NULL;
