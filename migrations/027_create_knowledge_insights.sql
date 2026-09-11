-- Derived knowledge summaries and tags are versioned separately from the
-- uploader's description and approval state.
CREATE TABLE IF NOT EXISTS knowledge_item_insights (
    id BIGSERIAL PRIMARY KEY,
    knowledge_item_id BIGINT NOT NULL,
    source_fingerprint VARCHAR(128) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'proposed',
    summary_markdown TEXT NOT NULL DEFAULT '',
    tags_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    model VARCHAR(128) NOT NULL DEFAULT '',
    prompt_version VARCHAR(64) NOT NULL DEFAULT '',
    review_due_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL,
    reviewed_by BIGINT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_knowledge_insight_status CHECK (status IN ('proposed', 'published', 'invalidated'))
);

CREATE INDEX IF NOT EXISTS idx_knowledge_insights_item ON knowledge_item_insights(knowledge_item_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_knowledge_insights_due ON knowledge_item_insights(status, review_due_at);

CREATE TABLE IF NOT EXISTS knowledge_recommendation_feedback (
    user_id BIGINT NOT NULL,
    knowledge_item_id BIGINT NOT NULL,
    helpful BOOLEAN NOT NULL,
    note VARCHAR(512) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (user_id, knowledge_item_id)
);
