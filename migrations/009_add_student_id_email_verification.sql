-- Add student_id and email columns to users table
ALTER TABLE users ADD COLUMN IF NOT EXISTS student_id VARCHAR(12) UNIQUE;
ALTER TABLE users ADD COLUMN IF NOT EXISTS email VARCHAR(256);

-- Create index on student_id
CREATE INDEX IF NOT EXISTS idx_users_student_id ON users(student_id);

-- Create verification_codes table
CREATE TABLE IF NOT EXISTS verification_codes (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(256) NOT NULL,
    code VARCHAR(6) NOT NULL,
    used BOOLEAN NOT NULL DEFAULT FALSE,
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create index on email
CREATE INDEX IF NOT EXISTS idx_verification_codes_email ON verification_codes(email);

-- Update seed data: update captain to use student ID
UPDATE users SET student_id = '202431060001', email = 'captain@swpucat.com' WHERE username = 'captain';
