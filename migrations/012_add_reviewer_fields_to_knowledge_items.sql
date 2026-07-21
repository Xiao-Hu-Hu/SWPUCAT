-- Add reviewer fields to knowledge_items table
ALTER TABLE knowledge_items ADD COLUMN IF NOT EXISTS reviewer_id BIGINT DEFAULT 0;
ALTER TABLE knowledge_items ADD COLUMN IF NOT EXISTS reviewer_name VARCHAR(32) DEFAULT '';

-- Create index on reviewer_id
CREATE INDEX IF NOT EXISTS idx_knowledge_items_reviewer_id ON knowledge_items(reviewer_id);
