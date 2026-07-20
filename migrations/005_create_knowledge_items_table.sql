-- Create knowledge_items table
CREATE TABLE IF NOT EXISTS knowledge_items (
    id BIGSERIAL PRIMARY KEY,
    type VARCHAR(8) NOT NULL,
    name VARCHAR(256) NOT NULL,
    url VARCHAR(512),
    file_size VARCHAR(32),
    file_key VARCHAR(256),
    category_id BIGINT NOT NULL,
    category_name VARCHAR(64),
    uploader_id BIGINT NOT NULL,
    uploader_name VARCHAR(32) NOT NULL,
    approved BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create index on category_id
CREATE INDEX IF NOT EXISTS idx_knowledge_items_category_id ON knowledge_items(category_id);

-- Create index on uploader_id
CREATE INDEX IF NOT EXISTS idx_knowledge_items_uploader_id ON knowledge_items(uploader_id);

-- Create index on approved
CREATE INDEX IF NOT EXISTS idx_knowledge_items_approved ON knowledge_items(approved);
