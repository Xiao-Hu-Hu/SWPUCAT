-- Track legacy category bindings and repeatable migration dry-runs. A dry-run
-- is an auditable proposal; it never changes report or knowledge records.
CREATE TABLE IF NOT EXISTS report_period_category_bindings (
    period_id BIGINT NOT NULL,
    category_id BIGINT NOT NULL,
    binding_type VARCHAR(16) NOT NULL DEFAULT 'legacy',
    confidence NUMERIC(5,4) NOT NULL DEFAULT 0,
    source VARCHAR(32) NOT NULL DEFAULT 'manual',
    confirmed_by BIGINT,
    confirmed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (period_id, category_id),
    CONSTRAINT chk_report_binding_type CHECK (binding_type IN ('legacy', 'primary', 'additional')),
    CONSTRAINT chk_report_binding_confidence CHECK (confidence >= 0 AND confidence <= 1)
);

CREATE INDEX IF NOT EXISTS idx_report_bindings_category_id ON report_period_category_bindings(category_id);

CREATE TABLE IF NOT EXISTS report_migration_runs (
    id BIGSERIAL PRIMARY KEY,
    mode VARCHAR(16) NOT NULL DEFAULT 'dry_run',
    status VARCHAR(16) NOT NULL DEFAULT 'created',
    filter_category_id BIGINT,
    started_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    finished_at TIMESTAMPTZ,
    created_by BIGINT,
    summary_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_report_migration_mode CHECK (mode IN ('dry_run', 'apply')),
    CONSTRAINT chk_report_migration_status CHECK (status IN ('created', 'running', 'succeeded', 'failed', 'needs_review'))
);

CREATE TABLE IF NOT EXISTS report_migration_items (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL,
    knowledge_item_id BIGINT NOT NULL,
    suggested_period_key VARCHAR(7) NOT NULL DEFAULT '',
    suggested_author_id BIGINT,
    suggested_author_name VARCHAR(32) NOT NULL DEFAULT '',
    decision VARCHAR(16) NOT NULL DEFAULT 'needs_review',
    confidence NUMERIC(5,4) NOT NULL DEFAULT 0,
    reason VARCHAR(512) NOT NULL DEFAULT '',
    target_report_id BIGINT,
    reviewed_by BIGINT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_report_migration_item UNIQUE (run_id, knowledge_item_id),
    CONSTRAINT chk_report_migration_decision CHECK (decision IN ('suggested', 'accepted', 'rejected', 'needs_review', 'excluded')),
    CONSTRAINT chk_report_migration_confidence CHECK (confidence >= 0 AND confidence <= 1)
);

CREATE INDEX IF NOT EXISTS idx_report_migration_items_run_id ON report_migration_items(run_id, decision);
CREATE INDEX IF NOT EXISTS idx_report_migration_items_knowledge_id ON report_migration_items(knowledge_item_id);
