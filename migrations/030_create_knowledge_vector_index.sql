CREATE EXTENSION IF NOT EXISTS vector;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id BIGSERIAL PRIMARY KEY,
    knowledge_item_id BIGINT NOT NULL REFERENCES knowledge_items(id) ON DELETE CASCADE,
    source_kind VARCHAR(24) NOT NULL,
    chunk_index INTEGER NOT NULL CHECK (chunk_index >= 0),
    content TEXT NOT NULL,
    content_hash VARCHAR(64) NOT NULL,
    embedding VECTOR,
    embedding_model VARCHAR(128) NOT NULL DEFAULT '',
    embedding_dimensions INTEGER NOT NULL DEFAULT 0 CHECK (embedding_dimensions >= 0),
    metadata_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_knowledge_chunk_embedding_dimensions CHECK (
        embedding IS NULL OR vector_dims(embedding) = embedding_dimensions
    ),
    CONSTRAINT uq_knowledge_chunk_source UNIQUE (
        knowledge_item_id,
        source_kind,
        chunk_index,
        content_hash,
        embedding_model
    )
);

CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_item
    ON knowledge_chunks(knowledge_item_id, chunk_index);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_embedding_model
    ON knowledge_chunks(embedding_model, embedding_dimensions)
    WHERE embedding IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_content_trgm
    ON knowledge_chunks USING GIN (content gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_knowledge_items_name_trgm
    ON knowledge_items USING GIN (name gin_trgm_ops)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_knowledge_items_description_trgm
    ON knowledge_items USING GIN (description gin_trgm_ops)
    WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_knowledge_categories_name_trgm
    ON knowledge_categories USING GIN (name gin_trgm_ops);

CREATE TABLE IF NOT EXISTS knowledge_index_jobs (
    knowledge_item_id BIGINT PRIMARY KEY REFERENCES knowledge_items(id) ON DELETE CASCADE,
    status VARCHAR(16) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    source_fingerprint VARCHAR(64) NOT NULL DEFAULT '',
    attempts INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    available_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    locked_at TIMESTAMPTZ,
    last_error VARCHAR(1000) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_knowledge_index_jobs_claim
    ON knowledge_index_jobs(status, available_at, knowledge_item_id);
