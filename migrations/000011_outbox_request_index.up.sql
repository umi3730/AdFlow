-- Settlement completion and dependent ingress share a request row. Limit the
-- subsequent Outbox update to that request instead of scanning the whole queue.
ALTER TABLE event_outbox ADD KEY idx_outbox_request_status (aggregate_key, status);
