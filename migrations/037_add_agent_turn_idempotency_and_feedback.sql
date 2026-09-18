-- Client-owned turn and generation identifiers make completed chat requests
-- replay-safe. Nullable columns preserve all history written before this
-- protocol existed; new application writes always populate all three fields.
ALTER TABLE agent_messages
    ADD COLUMN IF NOT EXISTS client_turn_id VARCHAR(32),
    ADD COLUMN IF NOT EXISTS generation_id VARCHAR(32),
    ADD COLUMN IF NOT EXISTS request_id VARCHAR(32);

ALTER TABLE agent_messages
    DROP CONSTRAINT IF EXISTS chk_agent_messages_client_turn_id,
    ADD CONSTRAINT chk_agent_messages_client_turn_id
        CHECK (client_turn_id IS NULL OR client_turn_id ~ '^[0-9a-f]{32}$'),
    DROP CONSTRAINT IF EXISTS chk_agent_messages_generation_id,
    ADD CONSTRAINT chk_agent_messages_generation_id
        CHECK (generation_id IS NULL OR generation_id ~ '^[0-9a-f]{32}$'),
    DROP CONSTRAINT IF EXISTS chk_agent_messages_request_id,
    ADD CONSTRAINT chk_agent_messages_request_id
        CHECK (request_id IS NULL OR request_id ~ '^[0-9a-f]{32}$');

CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_messages_user_turn
    ON agent_messages(conversation_id, client_turn_id)
    WHERE role = 'user' AND client_turn_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_messages_assistant_turn
    ON agent_messages(conversation_id, client_turn_id)
    WHERE role = 'assistant' AND client_turn_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_agent_messages_generation
    ON agent_messages(conversation_id, generation_id)
    WHERE generation_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS agent_response_feedback (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id BIGINT NOT NULL REFERENCES agent_messages(id) ON DELETE CASCADE,
    request_id VARCHAR(32) NOT NULL,
    helpful BOOLEAN NOT NULL,
    note VARCHAR(500) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_agent_response_feedback_user_message UNIQUE (user_id, message_id),
    CONSTRAINT chk_agent_response_feedback_request_id
        CHECK (request_id ~ '^[0-9a-f]{32}$')
);

CREATE INDEX IF NOT EXISTS idx_agent_response_feedback_request
    ON agent_response_feedback(request_id, created_at DESC, id DESC);
