CREATE TABLE event_receipts (
    event_id VARCHAR(128) NOT NULL,
    request_id VARCHAR(128) NOT NULL,
    campaign_id CHAR(32) NOT NULL,
    creative_id CHAR(32) NOT NULL,
    event_type VARCHAR(16) NOT NULL,
    value_fen BIGINT NOT NULL DEFAULT 0,
    occurred_at DATETIME(3) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'ACCEPTED',
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (event_id),
    KEY idx_event_receipts_request (request_id, occurred_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE event_outbox (
    event_id VARCHAR(128) NOT NULL,
    aggregate_key VARCHAR(128) NOT NULL,
    payload JSON NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
    attempts INT UNSIGNED NOT NULL DEFAULT 0,
    next_attempt_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    locked_by VARCHAR(64) NULL,
    locked_until DATETIME(3) NULL,
    published_at DATETIME(3) NULL,
    dead_lettered_at DATETIME(3) NULL,
    last_error VARCHAR(1024) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (event_id),
    KEY idx_event_outbox_claim (status, next_attempt_at, locked_until, created_at),
    CONSTRAINT fk_event_outbox_receipt FOREIGN KEY (event_id) REFERENCES event_receipts(event_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE processed_events (
    event_id VARCHAR(128) NOT NULL,
    request_id VARCHAR(128) NOT NULL,
    campaign_id CHAR(32) NOT NULL,
    creative_id CHAR(32) NOT NULL,
    event_type VARCHAR(16) NOT NULL,
    value_fen BIGINT NOT NULL DEFAULT 0,
    occurred_at DATETIME(3) NOT NULL,
    processed_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (event_id),
    KEY idx_processed_events_request_type (request_id, event_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE campaign_metrics (
    campaign_id CHAR(32) NOT NULL,
    impressions BIGINT UNSIGNED NOT NULL DEFAULT 0,
    clicks BIGINT UNSIGNED NOT NULL DEFAULT 0,
    conversions BIGINT UNSIGNED NOT NULL DEFAULT 0,
    value_fen BIGINT NOT NULL DEFAULT 0,
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (campaign_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
