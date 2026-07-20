-- Create checkin_records table
CREATE TABLE IF NOT EXISTS checkin_records (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL,
    type VARCHAR(8) NOT NULL,
    date VARCHAR(10) NOT NULL,
    time VARCHAR(8) NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Create index on user_id
CREATE INDEX IF NOT EXISTS idx_checkin_records_user_id ON checkin_records(user_id);

-- Create index on date
CREATE INDEX IF NOT EXISTS idx_checkin_records_date ON checkin_records(date);

-- Create composite index on user_id and date
CREATE INDEX IF NOT EXISTS idx_checkin_records_user_date ON checkin_records(user_id, date);
