-- Durable Agent conversations are scoped by both user and opaque session id.
-- Message text, tool outputs and citations stay in PostgreSQL so the Agent can
-- recover multi-turn context after a process restart without adding Redis.
CREATE TABLE IF NOT EXISTS agent_conversations (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    session_id VARCHAR(128) NOT NULL,
    title VARCHAR(256) NOT NULL DEFAULT '',
    rolling_summary TEXT NOT NULL DEFAULT '',
    last_sources_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    last_active_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_agent_conversations_owner_session UNIQUE (user_id, session_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_conversations_owner_active
    ON agent_conversations(user_id, last_active_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS agent_messages (
    id BIGSERIAL PRIMARY KEY,
    conversation_id BIGINT NOT NULL REFERENCES agent_conversations(id) ON DELETE CASCADE,
    role VARCHAR(16) NOT NULL,
    content TEXT NOT NULL,
    tool_name VARCHAR(128) NOT NULL DEFAULT '',
    tool_result TEXT NOT NULL DEFAULT '',
    citations_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    token_count INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_agent_message_role CHECK (role IN ('system', 'user', 'assistant', 'tool')),
    CONSTRAINT chk_agent_message_token_count CHECK (token_count >= 0)
);

CREATE INDEX IF NOT EXISTS idx_agent_messages_conversation_created
    ON agent_messages(conversation_id, created_at DESC, id DESC);
