-- Durable in-app notifications. Delivery is intentionally database-backed so
-- the first deployment does not require Redis or a message broker.
CREATE TABLE IF NOT EXISTS notifications (
    id BIGSERIAL PRIMARY KEY,
    recipient_id BIGINT NOT NULL,
    notification_type VARCHAR(32) NOT NULL,
    title VARCHAR(256) NOT NULL,
    body_markdown TEXT NOT NULL DEFAULT '',
    entity_type VARCHAR(32) NOT NULL DEFAULT '',
    entity_id BIGINT,
    dedupe_key VARCHAR(256) NOT NULL,
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    read_at TIMESTAMPTZ,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_notifications_recipient_dedupe UNIQUE (recipient_id, dedupe_key),
    CONSTRAINT chk_notifications_type CHECK (notification_type IN ('checkin_anomaly', 'report_due', 'evaluation_published'))
);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient_created ON notifications(recipient_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications(recipient_id, read_at, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_preferences (
    user_id BIGINT PRIMARY KEY,
    checkin_anomaly_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    report_due_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    evaluation_published_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);
