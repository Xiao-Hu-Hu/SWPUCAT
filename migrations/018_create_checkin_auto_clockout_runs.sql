CREATE TABLE IF NOT EXISTS checkin_auto_clockout_runs (
    id BIGSERIAL PRIMARY KEY,
    run_date VARCHAR(10) NOT NULL UNIQUE,
    status VARCHAR(16) NOT NULL,
    started_at TIMESTAMP NOT NULL,
    finished_at TIMESTAMP,
    processed INTEGER NOT NULL DEFAULT 0,
    error_message VARCHAR(512),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_checkin_auto_clockout_runs_status
    ON checkin_auto_clockout_runs(status);
