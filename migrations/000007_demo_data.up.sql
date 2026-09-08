CREATE TABLE bootstrap_seeds (
    seed_key VARCHAR(64) NOT NULL,
    completed_at DATETIME(3) NULL,
    PRIMARY KEY (seed_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
