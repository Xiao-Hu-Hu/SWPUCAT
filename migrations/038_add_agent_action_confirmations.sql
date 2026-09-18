-- Persist structured action cards with assistant messages so a refresh does
-- not turn a pending confirmation back into unstructured model text.
ALTER TABLE agent_messages
    ADD COLUMN IF NOT EXISTS actions_json JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE agent_messages
    DROP CONSTRAINT IF EXISTS chk_agent_messages_actions_json,
    ADD CONSTRAINT chk_agent_messages_actions_json
        CHECK (jsonb_typeof(actions_json) = 'array');

-- A token digest is claimed before executing the underlying write. The raw
-- signed token and authorization credentials are never stored.
CREATE TABLE IF NOT EXISTS agent_action_confirmations (
    token_hash CHAR(64) PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    session_id VARCHAR(128) NOT NULL,
    tool_name VARCHAR(128) NOT NULL,
    parameters_digest CHAR(64) NOT NULL,
    status VARCHAR(16) NOT NULL,
    result_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    error_code VARCHAR(64),
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    CONSTRAINT chk_agent_action_confirmation_hashes CHECK (
        token_hash ~ '^[0-9a-f]{64}$'
        AND parameters_digest ~ '^[0-9a-f]{64}$'
    ),
    CONSTRAINT chk_agent_action_confirmation_status
        CHECK (status IN ('executing', 'succeeded', 'failed')),
    CONSTRAINT chk_agent_action_confirmation_completion CHECK (
        (status = 'executing' AND completed_at IS NULL)
        OR (status IN ('succeeded', 'failed') AND completed_at IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_agent_action_confirmations_user_created
    ON agent_action_confirmations(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_agent_action_confirmations_expiry
    ON agent_action_confirmations(expires_at ASC);
