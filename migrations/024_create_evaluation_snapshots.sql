-- Immutable evidence snapshots for captain evaluation drafts.
CREATE TABLE IF NOT EXISTS evaluation_exceptions (
    id BIGSERIAL PRIMARY KEY,
    period_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    exception_type VARCHAR(32) NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    reason VARCHAR(512) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'pending',
    approved_by BIGINT,
    approved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_evaluation_exception_range CHECK (starts_on <= ends_on),
    CONSTRAINT chk_evaluation_exception_status CHECK (status IN ('pending', 'approved', 'rejected'))
);

CREATE INDEX IF NOT EXISTS idx_evaluation_exceptions_period_user ON evaluation_exceptions(period_id, user_id);
CREATE INDEX IF NOT EXISTS idx_evaluation_exceptions_status ON evaluation_exceptions(status);

CREATE TABLE IF NOT EXISTS evaluation_snapshots (
    id BIGSERIAL PRIMARY KEY,
    period_id BIGINT NOT NULL,
    period_key VARCHAR(7) NOT NULL,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    timezone VARCHAR(64) NOT NULL,
    cutoff_at TIMESTAMPTZ NOT NULL,
    roster_version BIGINT NOT NULL,
    rule_version VARCHAR(64) NOT NULL,
    source_watermark VARCHAR(128) NOT NULL DEFAULT '',
    snapshot_hash VARCHAR(128) NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_evaluation_snapshot_hash UNIQUE (snapshot_hash),
    CONSTRAINT chk_evaluation_snapshot_status CHECK (status IN ('draft', 'published', 'invalidated'))
);

CREATE INDEX IF NOT EXISTS idx_evaluation_snapshots_period ON evaluation_snapshots(period_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_evaluation_snapshots_status ON evaluation_snapshots(status);

CREATE TABLE IF NOT EXISTS evaluation_snapshot_members (
    snapshot_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    display_name VARCHAR(32) NOT NULL DEFAULT '',
    required BOOLEAN NOT NULL DEFAULT TRUE,
    exempt BOOLEAN NOT NULL DEFAULT FALSE,
    total_minutes NUMERIC(12,2) NOT NULL DEFAULT 0,
    total_hours NUMERIC(12,2) NOT NULL DEFAULT 0,
    attendance_days INTEGER NOT NULL DEFAULT 0,
    achieved_weeks INTEGER NOT NULL DEFAULT 0,
    anomalies_json JSONB NOT NULL DEFAULT '[]'::jsonb,
    report_status VARCHAR(16) NOT NULL DEFAULT 'unknown',
    report_version_id BIGINT,
    data_state VARCHAR(16) NOT NULL DEFAULT 'complete',
    unknown_reason VARCHAR(512) NOT NULL DEFAULT '',
    PRIMARY KEY (snapshot_id, user_id),
    CONSTRAINT chk_evaluation_member_report_status CHECK (report_status IN ('submitted', 'reviewed', 'missing', 'unparsed', 'unknown')),
    CONSTRAINT chk_evaluation_member_data_state CHECK (data_state IN ('complete', 'partial', 'unknown'))
);

CREATE INDEX IF NOT EXISTS idx_evaluation_snapshot_members_user ON evaluation_snapshot_members(user_id);
CREATE INDEX IF NOT EXISTS idx_evaluation_snapshot_members_report_status ON evaluation_snapshot_members(snapshot_id, report_status);
