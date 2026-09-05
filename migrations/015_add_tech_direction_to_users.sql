-- Add tech_direction to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS tech_direction VARCHAR(32) DEFAULT '';
