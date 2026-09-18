-- Persist privacy-filtered Agent execution audits separately from durable chat
-- messages. Prompts, answers, authorization headers and raw provider/tool
-- payloads are deliberately excluded from these tables.
CREATE TABLE IF NOT EXISTS agent_runs (
    id BIGSERIAL PRIMARY KEY,
    request_id VARCHAR(32) NOT NULL,
    user_id BIGINT,
    conversation_id BIGINT REFERENCES agent_conversations(id) ON DELETE SET NULL,
    session_id VARCHAR(128),
    actor_role VARCHAR(16),
    status VARCHAR(16) NOT NULL,
    telemetry_status VARCHAR(16) NOT NULL,
    http_status SMALLINT,
    failure_source VARCHAR(32),
    failure_category VARCHAR(32),
    failure_code VARCHAR(64),
    failure_retryable BOOLEAN,
    started_at TIMESTAMPTZ NOT NULL,
    finished_at TIMESTAMPTZ NOT NULL,
    duration_ms BIGINT NOT NULL,
    agent_started_at TIMESTAMPTZ,
    agent_finished_at TIMESTAMPTZ,
    agent_duration_ms BIGINT,
    conversation_persisted BOOLEAN,
    model_call_count INTEGER,
    tool_call_count INTEGER,
    dropped_model_calls INTEGER,
    dropped_tool_calls INTEGER,
    usage_reported_model_calls INTEGER,
    usage_missing_model_calls INTEGER,
    prompt_tokens BIGINT,
    completion_tokens BIGINT,
    total_tokens BIGINT,
    cached_tokens BIGINT,
    reasoning_tokens BIGINT,
    -- json (rather than jsonb) deliberately preserves the exact bounded
    -- projection bytes. The domain contract validates and counts those bytes
    -- before insert, so normalizing whitespace or key order would make a
    -- subsequently loaded aggregate fail the same contract.
    usage_json JSON,
    model_calls_json JSON,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_agent_runs_request_id UNIQUE (request_id),
    CONSTRAINT chk_agent_runs_request_id
        CHECK (request_id ~ '^[0-9a-f]{32}$'),
    CONSTRAINT chk_agent_runs_user_id
        CHECK (user_id IS NULL OR user_id > 0),
    CONSTRAINT chk_agent_runs_session_id
        CHECK (session_id IS NULL OR (user_id IS NOT NULL AND BTRIM(session_id) <> '')),
	CONSTRAINT chk_agent_runs_conversation_identity
		CHECK (conversation_id IS NULL OR (user_id IS NOT NULL AND session_id IS NOT NULL)),
    CONSTRAINT chk_agent_runs_actor_role
        CHECK (actor_role IS NULL OR (user_id IS NOT NULL AND actor_role IN ('member', 'captain', 'super_admin'))),
    CONSTRAINT chk_agent_runs_status
        CHECK (status IN ('success', 'rejected', 'busy', 'unavailable', 'timeout', 'canceled', 'error')),
    CONSTRAINT chk_agent_runs_telemetry_status
        CHECK (telemetry_status IN ('not_started', 'captured', 'unavailable', 'invalid')),
    CONSTRAINT chk_agent_runs_http_status
        CHECK (http_status IS NULL OR http_status BETWEEN 100 AND 599),
    CONSTRAINT chk_agent_runs_failure_fields
        CHECK (
			(failure_source IS NULL OR failure_source IN ('main', 'agent', 'transport', 'model', 'tool'))
            AND (failure_category IS NULL OR BTRIM(failure_category) <> '')
            AND (failure_code IS NULL OR BTRIM(failure_code) <> '')
			AND (
				(status = 'success' AND failure_source IS NULL AND failure_category IS NULL
					AND failure_code IS NULL AND failure_retryable IS NULL)
				OR
				(status <> 'success' AND failure_source IS NOT NULL AND failure_category IS NOT NULL
					AND failure_code IS NOT NULL AND failure_retryable IS NOT NULL)
			)
        ),
    CONSTRAINT chk_agent_runs_duration
        CHECK (
            duration_ms >= 0
            AND finished_at >= started_at
            -- PostgreSQL stores timestamptz at microsecond precision. Rounding
            -- each endpoint can move the truncated millisecond delta by one.
            AND ABS(
                duration_ms
                - FLOOR(EXTRACT(EPOCH FROM (finished_at - started_at)) * 1000)::BIGINT
            ) <= 1
        ),
    CONSTRAINT chk_agent_runs_agent_duration
        CHECK (agent_duration_ms IS NULL OR agent_duration_ms BETWEEN 0 AND 86400000),
    CONSTRAINT chk_agent_runs_counts
        CHECK (
            (model_call_count IS NULL OR model_call_count BETWEEN 0 AND 1000000)
            AND (tool_call_count IS NULL OR tool_call_count BETWEEN 0 AND 1000000)
            AND (dropped_model_calls IS NULL OR dropped_model_calls BETWEEN 0 AND 1000000)
            AND (dropped_tool_calls IS NULL OR dropped_tool_calls BETWEEN 0 AND 1000000)
            AND (usage_reported_model_calls IS NULL OR usage_reported_model_calls BETWEEN 0 AND 1000000)
            AND (usage_missing_model_calls IS NULL OR usage_missing_model_calls BETWEEN 0 AND 1000000)
			AND (dropped_model_calls IS NULL OR model_call_count IS NOT NULL AND dropped_model_calls <= model_call_count)
			AND (dropped_tool_calls IS NULL OR tool_call_count IS NOT NULL AND dropped_tool_calls <= tool_call_count)
			AND (usage_reported_model_calls IS NULL OR usage_missing_model_calls IS NULL
				OR model_call_count IS NOT NULL AND usage_reported_model_calls + usage_missing_model_calls = model_call_count)
        ),
    CONSTRAINT chk_agent_runs_telemetry_shape
        CHECK (
            (
                telemetry_status = 'not_started'
				AND model_call_count IS NOT DISTINCT FROM 0
				AND tool_call_count IS NOT DISTINCT FROM 0
				AND dropped_model_calls IS NOT DISTINCT FROM 0
				AND dropped_tool_calls IS NOT DISTINCT FROM 0
				AND usage_reported_model_calls IS NOT DISTINCT FROM 0
				AND usage_missing_model_calls IS NOT DISTINCT FROM 0
                AND usage_json IS NOT NULL
                AND json_typeof(usage_json) = 'object'
                AND model_calls_json IS NOT NULL
                AND json_typeof(model_calls_json) = 'array'
                AND json_array_length(model_calls_json) = 0
                AND agent_started_at IS NULL
                AND agent_finished_at IS NULL
                AND agent_duration_ms IS NULL
            )
            OR
            (
                telemetry_status = 'captured'
                AND model_call_count IS NOT NULL
                AND tool_call_count IS NOT NULL
                AND dropped_model_calls IS NOT NULL
                AND dropped_tool_calls IS NOT NULL
                AND usage_reported_model_calls IS NOT NULL
                AND usage_missing_model_calls IS NOT NULL
                AND usage_json IS NOT NULL
                AND json_typeof(usage_json) = 'object'
                AND model_calls_json IS NOT NULL
                AND json_typeof(model_calls_json) = 'array'
                AND agent_started_at IS NOT NULL
                AND agent_finished_at IS NOT NULL
                AND agent_duration_ms IS NOT NULL
            )
            OR
            (
                telemetry_status IN ('unavailable', 'invalid')
                AND model_call_count IS NULL
                AND tool_call_count IS NULL
                AND dropped_model_calls IS NULL
                AND dropped_tool_calls IS NULL
                AND usage_reported_model_calls IS NULL
                AND usage_missing_model_calls IS NULL
                AND usage_json IS NULL
                AND model_calls_json IS NULL
                AND agent_started_at IS NULL
                AND agent_finished_at IS NULL
                AND agent_duration_ms IS NULL
            )
        ),
    CONSTRAINT chk_agent_runs_complete_tokens
        CHECK (
			(prompt_tokens IS NULL OR prompt_tokens > 0)
			AND (completion_tokens IS NULL OR completion_tokens > 0)
			AND (total_tokens IS NULL OR total_tokens > 0)
			AND (cached_tokens IS NULL OR cached_tokens > 0)
			AND (reasoning_tokens IS NULL OR reasoning_tokens > 0)
			AND (
				(
					prompt_tokens IS NULL
					AND completion_tokens IS NULL
					AND total_tokens IS NULL
					AND cached_tokens IS NULL
					AND reasoning_tokens IS NULL
				)
				OR (
					telemetry_status = 'captured'
					AND usage_reported_model_calls IS NOT NULL
					AND usage_reported_model_calls > 0
					AND usage_missing_model_calls IS NOT DISTINCT FROM 0
				)
			)
        ),
	CONSTRAINT chk_agent_runs_model_call_totals
		CHECK (
			CASE
				WHEN telemetry_status = 'captured' AND json_typeof(model_calls_json) = 'array'
				THEN json_array_length(model_calls_json) + dropped_model_calls = model_call_count
				ELSE TRUE
			END
		),
    CONSTRAINT chk_agent_runs_usage_json
        CHECK (
            usage_json IS NULL
            OR (
                json_typeof(usage_json) = 'object'
                AND octet_length(usage_json::text) <= 16384
            )
        ),
    CONSTRAINT chk_agent_runs_model_calls_json
        CHECK (
            model_calls_json IS NULL
            OR (
                json_typeof(model_calls_json) = 'array'
                AND json_array_length(model_calls_json) <= 64
                AND octet_length(model_calls_json::text) <= 1048576
            )
        )
);

CREATE INDEX IF NOT EXISTS idx_agent_runs_owner_created
    ON agent_runs(user_id, created_at DESC, id DESC)
    WHERE user_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_agent_runs_owner_session_created
    ON agent_runs(user_id, session_id, created_at DESC, id DESC)
    WHERE user_id IS NOT NULL AND session_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_agent_runs_conversation_created
    ON agent_runs(conversation_id, created_at DESC, id DESC)
    WHERE conversation_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_agent_runs_failure_created
    ON agent_runs(status, created_at DESC, id DESC)
    WHERE status <> 'success';

CREATE TABLE IF NOT EXISTS agent_tool_calls (
    id BIGSERIAL PRIMARY KEY,
    run_id BIGINT NOT NULL REFERENCES agent_runs(id) ON DELETE CASCADE,
    sequence BIGINT NOT NULL,
    call_id VARCHAR(64),
    tool_name VARCHAR(128) NOT NULL,
    is_write BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(16) NOT NULL,
    started_at TIMESTAMPTZ NOT NULL,
    duration_ms BIGINT NOT NULL,
    arguments_json JSON NOT NULL,
    arguments_original_bytes BIGINT NOT NULL,
    arguments_captured_bytes INTEGER NOT NULL,
    arguments_redacted BOOLEAN NOT NULL,
    arguments_truncated BOOLEAN NOT NULL,
    result_json JSON NOT NULL,
    result_original_bytes BIGINT NOT NULL,
    result_captured_bytes INTEGER NOT NULL,
    result_redacted BOOLEAN NOT NULL,
    result_truncated BOOLEAN NOT NULL,
    error_category VARCHAR(32),
    error_code VARCHAR(64),
    error_type VARCHAR(64),
    error_http_status SMALLINT,
    error_retryable BOOLEAN,
    business_object_type VARCHAR(32),
    business_object_id BIGINT,
    business_object_status VARCHAR(32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT uq_agent_tool_calls_run_sequence UNIQUE (run_id, sequence),
    CONSTRAINT chk_agent_tool_calls_sequence
        CHECK (sequence > 0),
    CONSTRAINT chk_agent_tool_calls_call_id
        CHECK (call_id IS NULL OR call_id ~ '^call-[1-9][0-9]{0,19}$'),
    CONSTRAINT chk_agent_tool_calls_name
        CHECK (tool_name = '[invalid]' OR tool_name ~ '^[A-Za-z0-9_.-]{1,128}$'),
    CONSTRAINT chk_agent_tool_calls_status
        CHECK (status IN ('success', 'error', 'timeout', 'canceled', 'panic')),
    CONSTRAINT chk_agent_tool_calls_duration
        CHECK (duration_ms BETWEEN 0 AND 86400000),
    CONSTRAINT chk_agent_tool_calls_argument_sizes
        CHECK (
            arguments_original_bytes >= 0
            AND arguments_captured_bytes BETWEEN 0 AND 2048
            AND arguments_captured_bytes = octet_length(arguments_json::text)
            AND octet_length(arguments_json::text) <= 8192
        ),
    CONSTRAINT chk_agent_tool_calls_result_sizes
        CHECK (
            result_original_bytes >= 0
            AND result_captured_bytes BETWEEN 0 AND 4096
            AND result_captured_bytes = octet_length(result_json::text)
            AND octet_length(result_json::text) <= 16384
        ),
    CONSTRAINT chk_agent_tool_calls_error_fields
        CHECK (
            (error_category IS NULL OR BTRIM(error_category) <> '')
            AND (error_code IS NULL OR BTRIM(error_code) <> '')
            AND (error_type IS NULL OR BTRIM(error_type) <> '')
            AND (error_http_status IS NULL OR error_http_status BETWEEN 100 AND 599)
			AND (
				(status = 'success' AND error_category IS NULL AND error_code IS NULL
					AND error_type IS NULL AND error_http_status IS NULL AND error_retryable IS NULL)
				OR
				(status <> 'success' AND error_category IS NOT NULL AND error_code IS NOT NULL
					AND error_retryable IS NOT NULL)
			)
        ),
    CONSTRAINT chk_agent_tool_calls_business_object
        CHECK (
            (business_object_type IS NULL OR BTRIM(business_object_type) <> '')
            AND (business_object_id IS NULL OR business_object_id > 0)
            AND (business_object_status IS NULL OR BTRIM(business_object_status) <> '')
            AND (business_object_id IS NULL OR business_object_type IS NOT NULL)
            AND (business_object_status IS NULL OR business_object_type IS NOT NULL)
			AND (is_write OR (business_object_type IS NULL AND business_object_id IS NULL AND business_object_status IS NULL))
        )
);

CREATE INDEX IF NOT EXISTS idx_agent_tool_calls_name_started
    ON agent_tool_calls(tool_name, started_at DESC, id DESC);

CREATE INDEX IF NOT EXISTS idx_agent_tool_calls_write_started
    ON agent_tool_calls(started_at DESC, id DESC)
    WHERE is_write;

CREATE INDEX IF NOT EXISTS idx_agent_tool_calls_business_object
    ON agent_tool_calls(business_object_type, business_object_id, created_at DESC, id DESC)
    WHERE business_object_type IS NOT NULL AND business_object_id IS NOT NULL;
