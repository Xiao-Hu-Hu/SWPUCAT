-- Report periods, roster snapshots, report versions and knowledge assets.
-- These tables are intentionally separate from knowledge_categories: a
-- category is a reusable library location, while a report is a versioned
-- business record with an author, period and explicit evidence links.
CREATE TABLE IF NOT EXISTS report_periods (
    id BIGSERIAL PRIMARY KEY,
    period_key VARCHAR(7) NOT NULL UNIQUE,
    starts_on DATE NOT NULL,
    ends_on DATE NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
    due_at TIMESTAMPTZ,
    status VARCHAR(16) NOT NULL DEFAULT 'open',
    roster_version BIGINT NOT NULL DEFAULT 1,
    created_by BIGINT NOT NULL,
    created_by_name VARCHAR(32) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT chk_report_period_key CHECK (period_key ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
    CONSTRAINT chk_report_period_range CHECK (starts_on <= ends_on),
    CONSTRAINT chk_report_period_status CHECK (status IN ('open', 'closed', 'archived'))
);

CREATE INDEX IF NOT EXISTS idx_report_periods_status ON report_periods(status);
CREATE INDEX IF NOT EXISTS idx_report_periods_due_at ON report_periods(due_at);

CREATE TABLE IF NOT EXISTS report_period_members (
    period_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    display_name VARCHAR(32) NOT NULL DEFAULT '',
    required BOOLEAN NOT NULL DEFAULT TRUE,
    exempt BOOLEAN NOT NULL DEFAULT FALSE,
    exemption_reason VARCHAR(512) NOT NULL DEFAULT '',
    effective_from DATE,
    effective_until DATE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (period_id, user_id),
    CONSTRAINT chk_report_member_exemption CHECK (NOT exempt OR exemption_reason <> ''),
    CONSTRAINT chk_report_member_range CHECK (effective_from IS NULL OR effective_until IS NULL OR effective_from <= effective_until)
);

CREATE INDEX IF NOT EXISTS idx_report_period_members_user_id ON report_period_members(user_id);
CREATE INDEX IF NOT EXISTS idx_report_period_members_required ON report_period_members(period_id, required, exempt);

CREATE TABLE IF NOT EXISTS reports (
    id BIGSERIAL PRIMARY KEY,
    period_id BIGINT NOT NULL,
    report_type VARCHAR(16) NOT NULL DEFAULT 'monthly',
    author_id BIGINT NOT NULL,
    author_name VARCHAR(32) NOT NULL DEFAULT '',
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    visibility VARCHAR(16) NOT NULL DEFAULT 'private',
    current_version INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_reports_period_author_type UNIQUE (period_id, author_id, report_type),
    CONSTRAINT chk_reports_type CHECK (report_type IN ('monthly', 'team')),
    CONSTRAINT chk_reports_status CHECK (status IN ('draft', 'submitted', 'reviewed', 'archived')),
    CONSTRAINT chk_reports_visibility CHECK (visibility IN ('private', 'managers', 'shared')),
    CONSTRAINT chk_reports_current_version CHECK (current_version >= 0)
);

CREATE INDEX IF NOT EXISTS idx_reports_period_id ON reports(period_id);
CREATE INDEX IF NOT EXISTS idx_reports_author_id ON reports(author_id);
CREATE INDEX IF NOT EXISTS idx_reports_status ON reports(status);

CREATE TABLE IF NOT EXISTS report_versions (
    id BIGSERIAL PRIMARY KEY,
    report_id BIGINT NOT NULL,
    version INTEGER NOT NULL,
    status VARCHAR(16) NOT NULL DEFAULT 'draft',
    title VARCHAR(256) NOT NULL DEFAULT '',
    body_markdown TEXT NOT NULL DEFAULT '',
    content_hash VARCHAR(128) NOT NULL DEFAULT '',
    parse_status VARCHAR(16) NOT NULL DEFAULT 'pending',
    parse_error VARCHAR(512) NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL,
    submitted_by BIGINT,
    submitted_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_report_versions_report_version UNIQUE (report_id, version),
    CONSTRAINT chk_report_versions_number CHECK (version > 0),
    CONSTRAINT chk_report_versions_status CHECK (status IN ('draft', 'submitted', 'reviewed', 'rejected', 'invalidated')),
    CONSTRAINT chk_report_versions_parse_status CHECK (parse_status IN ('pending', 'processing', 'ready', 'failed'))
);

CREATE INDEX IF NOT EXISTS idx_report_versions_report_id ON report_versions(report_id, version DESC);
CREATE INDEX IF NOT EXISTS idx_report_versions_parse_status ON report_versions(parse_status);

CREATE TABLE IF NOT EXISTS report_version_assets (
    report_version_id BIGINT NOT NULL,
    knowledge_item_id BIGINT NOT NULL,
    asset_role VARCHAR(32) NOT NULL DEFAULT 'evidence',
    locator VARCHAR(512) NOT NULL DEFAULT '',
    note VARCHAR(512) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (report_version_id, knowledge_item_id, asset_role)
);

CREATE INDEX IF NOT EXISTS idx_report_version_assets_knowledge_item ON report_version_assets(knowledge_item_id);
