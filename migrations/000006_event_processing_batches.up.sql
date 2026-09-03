ALTER TABLE processed_events
    ADD COLUMN processing_batch_id CHAR(32) NULL AFTER processed_at,
    ADD KEY idx_processed_events_batch (processing_batch_id);
