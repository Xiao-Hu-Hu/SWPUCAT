-- Unify knowledge item lifecycle state while preserving the legacy booleans for
-- clients that have not migrated yet.
ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS status VARCHAR(16) NOT NULL DEFAULT 'pending';

ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

ALTER TABLE knowledge_items
    ADD COLUMN IF NOT EXISTS deleted_by BIGINT;

UPDATE knowledge_items
SET status = CASE
    WHEN rejected = TRUE THEN 'rejected'
    WHEN approved = TRUE THEN 'approved'
    ELSE 'pending'
END
WHERE status IS NULL OR status = '' OR status = 'pending';

UPDATE knowledge_items
SET approved = (status = 'approved'),
    rejected = (status = 'rejected');

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'chk_knowledge_items_status'
          AND conrelid = 'knowledge_items'::regclass
    ) THEN
        ALTER TABLE knowledge_items
            ADD CONSTRAINT chk_knowledge_items_status
            CHECK (status IN ('pending', 'approved', 'rejected', 'deleted'));
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_knowledge_items_status
    ON knowledge_items(status);

CREATE INDEX IF NOT EXISTS idx_knowledge_items_deleted_at
    ON knowledge_items(deleted_at);
