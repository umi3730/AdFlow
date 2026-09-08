-- Do not make quarantined events publishable during rollback.
UPDATE event_outbox SET status = 'RECONCILE', locked_by = NULL, locked_until = NULL,
    last_error = 'Settlement migration rolled back; reconciliation required'
WHERE status = 'SETTLING';
DROP TABLE event_settlements;
