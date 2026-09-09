-- Read-model indexes for bounded event-time reports. Existing event facts and
-- immutable settlement snapshots also make historical reports available.
ALTER TABLE processed_events
 ADD KEY idx_processed_report_time (occurred_at, campaign_id),
 ADD KEY idx_processed_report_campaign_time (campaign_id, occurred_at);
