-- Quality checks and human decisions are derived metadata. The original
-- report version remains immutable; accepted edits create a new version.
ALTER TABLE report_versions
    ADD COLUMN IF NOT EXISTS quality_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    ADD COLUMN IF NOT EXISTS quality_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS quality_checked_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_report_versions_quality_status ON report_versions(quality_status);

CREATE TABLE IF NOT EXISTS report_quality_decisions (
    report_version_id BIGINT NOT NULL,
    finding_index INTEGER NOT NULL,
    decision VARCHAR(16) NOT NULL,
    note VARCHAR(512) NOT NULL DEFAULT '',
    decided_by BIGINT NOT NULL,
    decided_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (report_version_id, finding_index),
    CONSTRAINT chk_report_quality_decision CHECK (decision IN ('accepted', 'rejected')),
    CONSTRAINT chk_report_quality_finding_index CHECK (finding_index >= 0)
);
