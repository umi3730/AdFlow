CREATE TABLE audit_logs (
    id CHAR(32) NOT NULL,
    actor_id VARCHAR(128) NOT NULL,
    actor_name VARCHAR(64) NOT NULL,
    actor_role VARCHAR(16) NOT NULL,
    action VARCHAR(64) NOT NULL,
    resource_type VARCHAR(32) NOT NULL,
    resource_id VARCHAR(128) NULL,
    request_id VARCHAR(128) NOT NULL,
    outcome VARCHAR(16) NOT NULL,
    metadata JSON NOT NULL,
    created_at DATETIME(3) NOT NULL,
    PRIMARY KEY (id),
    KEY idx_audit_logs_created (created_at, id),
    KEY idx_audit_logs_actor_created (actor_id, created_at),
    KEY idx_audit_logs_resource_created (resource_type, resource_id, created_at),
    KEY idx_audit_logs_request (request_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
