-- Create approvals table
CREATE TABLE IF NOT EXISTS approvals (
    id BIGSERIAL PRIMARY KEY,
    file_name VARCHAR(256) NOT NULL,
    file_size VARCHAR(32),
    file_key VARCHAR(256),
    category_id BIGINT NOT NULL,
    uploader_id BIGINT NOT NULL,
    uploader_name VARCHAR(32) NOT NULL,
    reviewer_id BIGINT,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create index on uploader_id
CREATE INDEX IF NOT EXISTS idx_approvals_uploader_id ON approvals(uploader_id);

-- Create index on status
CREATE INDEX IF NOT EXISTS idx_approvals_status ON approvals(status);
