ALTER TABLE agent_conversations
    ADD COLUMN IF NOT EXISTS title_customized BOOLEAN NOT NULL DEFAULT FALSE;

UPDATE agent_conversations
SET title_customized = TRUE
WHERE BTRIM(title) <> '' AND title <> '新会话';
