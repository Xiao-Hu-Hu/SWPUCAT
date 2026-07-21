-- Create invitation_codes table
CREATE TABLE IF NOT EXISTS invitation_codes (
    id BIGSERIAL PRIMARY KEY,
    code VARCHAR(6) NOT NULL UNIQUE,
    type VARCHAR(16) NOT NULL,
    creator_id BIGINT NOT NULL,
    used_by BIGINT,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_invitation_codes_code ON invitation_codes(code);
CREATE INDEX IF NOT EXISTS idx_invitation_codes_creator_id ON invitation_codes(creator_id);
CREATE INDEX IF NOT EXISTS idx_invitation_codes_used_by ON invitation_codes(used_by);

-- Insert default super admin user (password: admin123)
-- Password hash is bcrypt of 'admin123'
INSERT INTO users (username, student_id, email, password_hash, nickname, role, checkin_count, created_at, updated_at)
VALUES (
    '202400000001',
    '202400000001',
    'admin@swpucat.com',
    '$2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy',
    '超级管理员',
    'super_admin',
    0,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
)
ON CONFLICT (username) DO UPDATE SET role = 'super_admin';
