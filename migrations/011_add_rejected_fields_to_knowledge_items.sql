-- Add rejected and reject_reason columns to knowledge_items table
ALTER TABLE knowledge_items ADD COLUMN IF NOT EXISTS rejected BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE knowledge_items ADD COLUMN IF NOT EXISTS reject_reason VARCHAR(512);

-- Create index on rejected
CREATE INDEX IF NOT EXISTS idx_knowledge_items_rejected ON knowledge_items(rejected);
