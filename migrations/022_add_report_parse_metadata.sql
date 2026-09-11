-- Deterministic parser output is derived from a report version and can be
-- regenerated. It never replaces the submitted Markdown body.
ALTER TABLE report_versions
    ADD COLUMN IF NOT EXISTS parser_version VARCHAR(32) NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS parsed_json JSONB NOT NULL DEFAULT '{}'::jsonb,
    ADD COLUMN IF NOT EXISTS parsed_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_report_versions_parsed_at ON report_versions(parsed_at);
