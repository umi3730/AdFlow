ALTER TABLE decisions ADD COLUMN request_fingerprint CHAR(64) NULL;
CREATE TABLE decision_requests (
    request_id VARCHAR(128) NOT NULL,
    request_fingerprint CHAR(64) NOT NULL,
    owner_id VARCHAR(64) NULL,
    lease_until DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
