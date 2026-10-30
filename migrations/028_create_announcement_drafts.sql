-- Announcement drafts keep Agent-generated copy separate from published
-- announcements. External email is opt-in and recorded on the draft.
CREATE TABLE IF NOT EXISTS announcement_drafts (
    id BIGSERIAL PRIMARY KEY,
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    title VARCHAR(256) NOT NULL,
    content TEXT NOT NULL,
    pinned BOOLEAN NOT NULL DEFAULT FALSE,
    source_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    recipient_ids_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    email_requested BOOLEAN NOT NULL DEFAULT FALSE,
    email_result_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    announcement_id BIGINT,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    confirmed_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ,
    CONSTRAINT chk_announcement_draft_status CHECK (status IN ('draft', 'confirmed', 'published', 'discarded'))
);

CREATE INDEX IF NOT EXISTS idx_announcement_drafts_status ON announcement_drafts(status, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_announcement_drafts_creator ON announcement_drafts(created_by, updated_at DESC);

CREATE TABLE IF NOT EXISTS announcement_draft_events (
    id BIGSERIAL PRIMARY KEY,
    draft_id BIGINT NOT NULL,
    actor_id BIGINT NOT NULL,
    event_type VARCHAR(32) NOT NULL,
    detail_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_announcement_draft_events_draft ON announcement_draft_events(draft_id, created_at DESC);
