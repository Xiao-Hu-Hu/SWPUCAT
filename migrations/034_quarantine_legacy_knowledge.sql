-- Add a reversible quarantine state. Quarantined rows and attachments remain
-- intact, while normal list/search/download paths exclude them because they
-- are no longer approved. Every transition is recorded append-only.
ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS quarantined_at TIMESTAMPTZ;

ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS quarantined_by BIGINT NOT NULL DEFAULT 0;

ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS quarantine_reason_code VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS quarantine_reason VARCHAR(512) NOT NULL DEFAULT '';

ALTER TABLE knowledge_items
    DROP CONSTRAINT IF EXISTS chk_knowledge_items_status;

ALTER TABLE knowledge_items
    ADD CONSTRAINT chk_knowledge_items_status
    CHECK (status IN ('pending', 'approved', 'rejected', 'quarantined', 'deleted'));

CREATE INDEX IF NOT EXISTS idx_knowledge_items_quarantined_at
    ON knowledge_items(quarantined_at);

CREATE TABLE IF NOT EXISTS knowledge_item_lifecycle_events (
    id BIGSERIAL PRIMARY KEY,
    item_id BIGINT NOT NULL REFERENCES knowledge_items(id) ON DELETE RESTRICT,
    event_type VARCHAR(32) NOT NULL,
    from_status VARCHAR(16) NOT NULL,
    to_status VARCHAR(16) NOT NULL,
    actor_id BIGINT NOT NULL,
    actor_name VARCHAR(32) NOT NULL,
    reason_code VARCHAR(64) NOT NULL,
    reason VARCHAR(512) NOT NULL,
    source_key VARCHAR(128),
    revision INTEGER NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_knowledge_lifecycle_event_type
        CHECK (event_type IN ('quarantine', 'restore')),
    CONSTRAINT chk_knowledge_lifecycle_from_status
        CHECK (from_status IN ('pending', 'approved', 'rejected', 'quarantined', 'deleted')),
    CONSTRAINT chk_knowledge_lifecycle_to_status
        CHECK (to_status IN ('pending', 'approved', 'rejected', 'quarantined', 'deleted')),
    CONSTRAINT chk_knowledge_lifecycle_transition
        CHECK (
            (event_type = 'quarantine' AND from_status = 'approved' AND to_status = 'quarantined')
            OR
            (event_type = 'restore' AND from_status = 'quarantined' AND to_status = 'approved')
        ),
    CONSTRAINT chk_knowledge_lifecycle_revision CHECK (revision > 0)
);

CREATE INDEX IF NOT EXISTS idx_knowledge_lifecycle_item_created
    ON knowledge_item_lifecycle_events(item_id, created_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_knowledge_lifecycle_actor
    ON knowledge_item_lifecycle_events(actor_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_knowledge_lifecycle_event_type
    ON knowledge_item_lifecycle_events(event_type, created_at DESC);

CREATE OR REPLACE FUNCTION reject_knowledge_lifecycle_event_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'knowledge lifecycle events are append-only';
END;
$$;

DROP TRIGGER IF EXISTS trg_knowledge_lifecycle_events_append_only
    ON knowledge_item_lifecycle_events;

CREATE TRIGGER trg_knowledge_lifecycle_events_append_only
    BEFORE UPDATE OR DELETE ON knowledge_item_lifecycle_events
    FOR EACH ROW
    EXECUTE FUNCTION reject_knowledge_lifecycle_event_mutation();

DROP TRIGGER IF EXISTS trg_knowledge_lifecycle_events_no_truncate
    ON knowledge_item_lifecycle_events;

CREATE TRIGGER trg_knowledge_lifecycle_events_no_truncate
    BEFORE TRUNCATE ON knowledge_item_lifecycle_events
    FOR EACH STATEMENT
    EXECUTE FUNCTION reject_knowledge_lifecycle_event_mutation();

WITH actor AS (
    SELECT
        COALESCE((SELECT id FROM users WHERE role = 'super_admin' ORDER BY id LIMIT 1), 0) AS id,
        COALESCE((SELECT nickname FROM users WHERE role = 'super_admin' ORDER BY id LIMIT 1), '系统知识治理') AS name
), targets(id, type, name, url, file_key, description_md5, reason_code, reason) AS (
    VALUES
        (7::BIGINT, 'link', 'Apple Music', 'https://music.apple.com/cn/new', '', '85cb939267f1f57f2337fce44e2ac964', 'out_of_scope', '泛娱乐外链不属于工作室正式知识目录，待内容负责人确认后再决定是否恢复'),
        (8::BIGINT, 'link', '抖音', 'https://www.douyin.com/jingxuan', '', '02751c01351e5f478c2317f522c0fe02', 'out_of_scope', '泛娱乐外链不属于工作室正式知识目录，待内容负责人确认后再决定是否恢复'),
        (9::BIGINT, 'file', 'Typora.zip', '', '2026-09-15/1789453954736616421.zip', '1098ed816f49fc7bbffdf297f93ad097', 'license_risk', '资料说明宣称提供已破解软件，版权与许可证状态不明'),
        (13::BIGINT, 'file', '7月总结.md', '', '2026-09-15/1789478091748329551.md', '518430bbb642d258f636d0e687febb42', 'misclassified_report', '个人月度总结应进入月报域，不应作为所有成员可检索的公共知识'),
        (14::BIGINT, 'file', '七月总结.md', '', '2026-09-15/1789478103799879636.md', '1aea98a05a18bb2dfadbd7cb1f54d28c', 'misclassified_report', '个人月度总结应进入月报域，不应作为所有成员可检索的公共知识')
), candidates AS MATERIALIZED (
    SELECT item.id, item.source_key, item.revision, target.reason_code, target.reason
    FROM knowledge_items item
    JOIN targets target ON target.id = item.id
    WHERE item.status = 'approved'
      AND item.source_origin = 'user'
      AND item.type = target.type
      AND item.name = target.name
      AND COALESCE(item.url, '') = target.url
      AND COALESCE(item.file_key, '') = target.file_key
      AND MD5(COALESCE(item.description, '')) = target.description_md5
      AND item.deleted_at IS NULL
    FOR UPDATE OF item
), logged AS (
    INSERT INTO knowledge_item_lifecycle_events (
        item_id, event_type, from_status, to_status,
        actor_id, actor_name, reason_code, reason,
        source_key, revision, created_at
    )
    SELECT
        candidate.id, 'quarantine', 'approved', 'quarantined',
        actor.id, actor.name, candidate.reason_code, candidate.reason,
        candidate.source_key, candidate.revision, CURRENT_TIMESTAMP
    FROM candidates candidate
    CROSS JOIN actor
    RETURNING item_id
)
UPDATE knowledge_items item
SET status = 'quarantined',
    approved = FALSE,
    rejected = FALSE,
    quarantined_at = CURRENT_TIMESTAMP,
    quarantined_by = actor.id,
    quarantine_reason_code = candidate.reason_code,
    quarantine_reason = candidate.reason
FROM candidates candidate
CROSS JOIN actor
WHERE item.id = candidate.id
  AND EXISTS (SELECT 1 FROM logged WHERE logged.item_id = item.id);
