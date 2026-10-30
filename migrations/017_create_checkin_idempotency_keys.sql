CREATE TABLE IF NOT EXISTS checkin_idempotency_keys (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    action VARCHAR(8) NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    record_id BIGINT NOT NULL,
    record_date VARCHAR(10) NOT NULL,
    record_time VARCHAR(8) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_checkin_idempotency_user_action_key
        UNIQUE (user_id, action, idempotency_key)
);

CREATE INDEX IF NOT EXISTS idx_checkin_idempotency_created_at
    ON checkin_idempotency_keys(created_at);
