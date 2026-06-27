-- Retention index for processed outbox events (cleanup job uses processed_at).
CREATE INDEX IF NOT EXISTS idx_outbox_events_processed_at
    ON outbox_events (processed_at)
    WHERE processed_at IS NOT NULL;
