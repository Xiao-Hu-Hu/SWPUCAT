-- Captain evaluation drafts are derived from an immutable evidence snapshot.
CREATE TABLE IF NOT EXISTS evaluation_drafts (
    id BIGSERIAL PRIMARY KEY,
    snapshot_id BIGINT NOT NULL,
    period_id BIGINT NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    audience VARCHAR(16) NOT NULL DEFAULT 'captain',
    title VARCHAR(256) NOT NULL DEFAULT '',
    summary_markdown TEXT NOT NULL DEFAULT '',
    model VARCHAR(128) NOT NULL DEFAULT '',
    prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    published_at TIMESTAMPTZ,
    CONSTRAINT chk_evaluation_draft_status CHECK (status IN ('draft', 'reviewed', 'published', 'invalidated')),
    CONSTRAINT chk_evaluation_draft_audience CHECK (audience IN ('captain', 'team', 'private'))
);

CREATE INDEX IF NOT EXISTS idx_evaluation_drafts_snapshot ON evaluation_drafts(snapshot_id);
CREATE INDEX IF NOT EXISTS idx_evaluation_drafts_period_status ON evaluation_drafts(period_id, status);

CREATE TABLE IF NOT EXISTS evaluation_draft_members (
    draft_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    display_name VARCHAR(32) NOT NULL DEFAULT '',
    comment_markdown TEXT NOT NULL DEFAULT '',
    recommendation VARCHAR(512) NOT NULL DEFAULT '',
    evidence_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    manual_override BOOLEAN NOT NULL DEFAULT FALSE,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (draft_id, user_id)
);

CREATE TABLE IF NOT EXISTS evaluation_draft_edits (
    id BIGSERIAL PRIMARY KEY,
    draft_id BIGINT NOT NULL,
    editor_id BIGINT NOT NULL,
    field VARCHAR(64) NOT NULL,
    old_value TEXT NOT NULL DEFAULT '',
    new_value TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_evaluation_draft_edits_draft ON evaluation_draft_edits(draft_id, created_at DESC);
