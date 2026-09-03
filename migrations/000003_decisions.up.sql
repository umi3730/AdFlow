CREATE TABLE decisions (
    request_id VARCHAR(128) NOT NULL,
    user_id VARCHAR(128) NOT NULL,
    slot_id VARCHAR(64) NOT NULL,
    matched BOOLEAN NOT NULL,
    campaign_id CHAR(32) NULL,
    creative_id CHAR(32) NULL,
    reservation_token VARCHAR(128) NULL,
    expires_at DATETIME(3) NULL,
    reason VARCHAR(32) NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (request_id),
    KEY idx_decisions_campaign_created (campaign_id, created_at),
    KEY idx_decisions_user_created (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
