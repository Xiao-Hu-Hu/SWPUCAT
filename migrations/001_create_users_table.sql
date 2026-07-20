-- Create users table
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(32) NOT NULL UNIQUE,
    password_hash VARCHAR(256) NOT NULL,
    nickname VARCHAR(32) NOT NULL,
    role VARCHAR(16) NOT NULL DEFAULT 'member',
    checkin_count BIGINT NOT NULL DEFAULT 0,
    last_checkin_date VARCHAR(10),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create index on username
CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);

-- Create index on role
CREATE INDEX IF NOT EXISTS idx_users_role ON users(role);
