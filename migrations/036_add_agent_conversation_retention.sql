-- Deletion tombstones keep only a one-way digest of the opaque session handle.
-- They prevent an explicitly deleted conversation from being silently recreated
-- while avoiding retention of its title, summary, messages, citations or raw id.
CREATE TABLE IF NOT EXISTS agent_conversation_tombstones (
    user_id BIGINT NOT NULL,
    session_hash CHAR(64) NOT NULL,
    reason VARCHAR(16) NOT NULL,
    deleted_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, session_hash),
    CONSTRAINT chk_agent_conversation_tombstone_hash
        CHECK (session_hash ~ '^[0-9a-f]{64}$'),
    CONSTRAINT chk_agent_conversation_tombstone_reason
        CHECK (reason IN ('user', 'retention'))
);

CREATE INDEX IF NOT EXISTS idx_agent_conversation_tombstones_deleted
    ON agent_conversation_tombstones(deleted_at DESC);

-- The existing owner-first index cannot support a global retention scan.
CREATE INDEX IF NOT EXISTS idx_agent_conversations_retention
    ON agent_conversations(last_active_at ASC, id ASC);
