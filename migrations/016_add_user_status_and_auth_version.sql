ALTER TABLE users
    ADD COLUMN IF NOT EXISTS status VARCHAR(16) NOT NULL DEFAULT 'active',
    ADD COLUMN IF NOT EXISTS auth_version BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN IF NOT EXISTS disabled_at TIMESTAMPTZ;

UPDATE users SET status = 'active' WHERE status IS NULL OR status = '';
UPDATE users SET auth_version = 1 WHERE auth_version IS NULL OR auth_version < 1;

CREATE INDEX IF NOT EXISTS idx_users_status ON users(status);
