CREATE TABLE campaigns (
    id CHAR(32) NOT NULL,
    name VARCHAR(128) NOT NULL,
    status VARCHAR(16) NOT NULL,
    slot_id VARCHAR(64) NOT NULL,
    start_at DATETIME(3) NOT NULL,
    end_at DATETIME(3) NOT NULL,
    active_version INT UNSIGNED NULL,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_campaigns_slot_status (slot_id, status),
    CONSTRAINT chk_campaign_period CHECK (end_at > start_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE campaign_versions (
    id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    campaign_id CHAR(32) NOT NULL,
    version INT UNSIGNED NOT NULL,
    targeting_rule JSON NOT NULL,
    daily_budget BIGINT UNSIGNED NOT NULL,
    impression_cost BIGINT UNSIGNED NOT NULL,
    frequency_limit INT UNSIGNED NOT NULL,
    published_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    UNIQUE KEY uk_campaign_version (campaign_id, version),
    CONSTRAINT fk_campaign_versions_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE creatives (
    id CHAR(32) NOT NULL,
    campaign_id CHAR(32) NOT NULL,
    title VARCHAR(128) NOT NULL,
    description VARCHAR(512) NOT NULL DEFAULT '',
    image_url VARCHAR(1024) NOT NULL,
    landing_url VARCHAR(1024) NOT NULL,
    status VARCHAR(16) NOT NULL,
    revision BIGINT UNSIGNED NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),
    PRIMARY KEY (id),
    KEY idx_creatives_campaign_status (campaign_id, status),
    CONSTRAINT fk_creatives_campaign FOREIGN KEY (campaign_id) REFERENCES campaigns(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
