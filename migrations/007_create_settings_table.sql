-- Create settings table
CREATE TABLE IF NOT EXISTS settings (
    id BIGSERIAL PRIMARY KEY,
    key VARCHAR(64) NOT NULL UNIQUE,
    value TEXT NOT NULL
);

-- Create index on key
CREATE INDEX IF NOT EXISTS idx_settings_key ON settings(key);
