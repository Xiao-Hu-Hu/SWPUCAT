package repository

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"SWPUCAT/internal/domain/agentaudit"
	"SWPUCAT/internal/infrastructure/database"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

const agentAuditPostgresTestDSNEnv = "SWPUCAT_TEST_POSTGRES_DSN"

// TestAgentAuditRepoPostgresFirstWriteReplayStable is deliberately opt-in: it
// writes to, and then removes its own rows from, a PostgreSQL 16 database that
// has already been migrated through 035_create_agent_audits.sql. Point the DSN
// only at a disposable database whose name starts with "swpucat_verify_".
func TestAgentAuditRepoPostgresFirstWriteReplayStable(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(agentAuditPostgresTestDSNEnv))
	if dsn == "" {
		t.Skipf("set %s to a disposable PostgreSQL 16 database migrated through 035", agentAuditPostgresTestDSNEnv)
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open PostgreSQL test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get PostgreSQL connection: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL test database: %v", err)
	}

	var databaseName, serverVersion string
	if err := db.WithContext(ctx).Raw(`SELECT current_database(), current_setting('server_version_num')`).Row().Scan(&databaseName, &serverVersion); err != nil {
		t.Fatalf("inspect PostgreSQL test database: %v", err)
	}
	if !strings.HasPrefix(databaseName, "swpucat_verify_") {
		t.Fatalf("refusing to write to database %q; test database name must start with swpucat_verify_", databaseName)
	}
	versionNumber, err := strconv.Atoi(serverVersion)
	if err != nil || versionNumber < 160000 || versionNumber >= 170000 {
		t.Fatalf("PostgreSQL server_version_num = %q, want 16.x", serverVersion)
	}
	var migrationCount int64
	if err := db.WithContext(ctx).Table("schema_migrations").
		Where("version = ?", "035_create_agent_audits.sql").Count(&migrationCount).Error; err != nil {
		t.Fatalf("read schema_migrations: %v", err)
	}
	if migrationCount != 1 {
		t.Fatalf("035 migration count = %d, want 1", migrationCount)
	}
	assertPostgresAgentAuditSchema(t, db, ctx)

	run := postgresRoundTripAuditRun(t)
	requestID := randomAgentAuditRequestID(t)
	run.RequestID = requestID
	repo := NewAgentAuditRepo(db)
	t.Cleanup(func() {
		result := db.Where("request_id = ?", requestID).Delete(&database.AgentRunModel{})
		if result.Error != nil {
			t.Errorf("clean up PostgreSQL audit row %s: %v", requestID, result.Error)
		}
	})

	originalStartedAt := run.StartedAt
	wantUsageJSON := *run.UsageJSON
	wantModelCallsJSON := *run.ModelCallsJSON
	wantPayloads := make([][2]string, len(run.ToolCalls))
	for i := range run.ToolCalls {
		wantPayloads[i] = [2]string{run.ToolCalls[i].ArgumentsJSON, run.ToolCalls[i].ResultJSON}
	}

	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("first CreateRun() error = %v", err)
	}
	if run.ID == 0 || len(run.ToolCalls) != 2 {
		t.Fatalf("first CreateRun() returned incomplete aggregate: %#v", run)
	}
	if run.StartedAt.Nanosecond()%1000 != 0 || run.FinishedAt.Nanosecond()%1000 != 0 || run.CreatedAt.Nanosecond()%1000 != 0 {
		t.Fatalf("first CreateRun() did not return PostgreSQL-normalized timestamps: start=%v finish=%v created=%v", run.StartedAt, run.FinishedAt, run.CreatedAt)
	}
	if run.StartedAt.Equal(originalStartedAt) {
		t.Fatalf("first CreateRun() returned the unrounded input timestamp %v", originalStartedAt)
	}
	if run.DurationMS != 0 || run.FinishedAt.Sub(run.StartedAt).Milliseconds() != 1 {
		t.Fatalf("precision-boundary duration = stored %dms, wall clock %dms; want 0ms and 1ms", run.DurationMS, run.FinishedAt.Sub(run.StartedAt).Milliseconds())
	}
	assertPostgresAuditPayloads(t, run, wantUsageJSON, wantModelCallsJSON, wantPayloads)

	firstResult := cloneAgentAuditRunForPostgresTest(run)
	if err := repo.CreateRun(ctx, run); err != nil {
		t.Fatalf("retry CreateRun() error = %v", err)
	}
	if !reflect.DeepEqual(run, firstResult) {
		t.Fatalf("retry did not return the exact first database aggregate\nfirst: %#v\nretry: %#v", firstResult, run)
	}
	assertPostgresAuditPayloads(t, run, wantUsageJSON, wantModelCallsJSON, wantPayloads)

	var runRows, toolRows int64
	if err := db.WithContext(ctx).Model(&database.AgentRunModel{}).Where("request_id = ?", requestID).Count(&runRows).Error; err != nil {
		t.Fatalf("count persisted agent run: %v", err)
	}
	if err := db.WithContext(ctx).Model(&database.AgentToolCallModel{}).Where("run_id = ?", run.ID).Count(&toolRows).Error; err != nil {
		t.Fatalf("count persisted tool calls: %v", err)
	}
	if runRows != 1 || toolRows != 2 {
		t.Fatalf("idempotent persistence counts = runs %d, tools %d; want 1 and 2", runRows, toolRows)
	}
	assertPostgresConstraintError(t,
		db.WithContext(ctx).Model(&database.AgentToolCallModel{}).Where("id = ?", run.ToolCalls[0].ID).
			UpdateColumn("arguments_captured_bytes", gorm.Expr("arguments_captured_bytes + 1")).Error,
		"chk_agent_tool_calls_argument_sizes",
	)
	assertPostgresConstraintError(t,
		db.WithContext(ctx).Model(&database.AgentToolCallModel{}).Where("id = ?", run.ToolCalls[0].ID).
			UpdateColumn("result_captured_bytes", gorm.Expr("result_captured_bytes + 1")).Error,
		"chk_agent_tool_calls_result_sizes",
	)
	assertPostgresConstraintError(t,
		db.WithContext(ctx).Model(&database.AgentRunModel{}).Where("id = ?", run.ID).
			UpdateColumn("duration_ms", gorm.Expr("duration_ms + 3")).Error,
		"chk_agent_runs_duration",
	)
}

func assertPostgresAgentAuditSchema(t *testing.T, db *gorm.DB, ctx context.Context) {
	t.Helper()
	type columnType struct {
		TableName  string
		ColumnName string
		DataType   string
	}
	var columns []columnType
	if err := db.WithContext(ctx).Raw(`
		SELECT table_name, column_name, data_type
		FROM information_schema.columns
		WHERE table_schema = 'public'
		  AND (table_name, column_name) IN (
			('agent_runs', 'usage_json'),
			('agent_runs', 'model_calls_json'),
			('agent_tool_calls', 'arguments_json'),
			('agent_tool_calls', 'result_json')
		  )`).Scan(&columns).Error; err != nil {
		t.Fatalf("read agent audit JSON column types: %v", err)
	}
	if len(columns) != 4 {
		t.Fatalf("agent audit JSON column count = %d, want 4", len(columns))
	}
	for _, column := range columns {
		if column.DataType != "json" {
			t.Fatalf("%s.%s type = %s, want json", column.TableName, column.ColumnName, column.DataType)
		}
	}

	expectedConstraints := []string{
		"uq_agent_runs_request_id",
		"chk_agent_runs_duration",
		"chk_agent_runs_telemetry_shape",
		"chk_agent_runs_model_call_totals",
		"uq_agent_tool_calls_run_sequence",
		"chk_agent_tool_calls_argument_sizes",
		"chk_agent_tool_calls_result_sizes",
	}
	for _, name := range expectedConstraints {
		var count int64
		if err := db.WithContext(ctx).Raw(`
			SELECT COUNT(*)
			FROM pg_constraint AS constraint_definition
			JOIN pg_class AS relation ON relation.oid = constraint_definition.conrelid
			JOIN pg_namespace AS namespace ON namespace.oid = relation.relnamespace
			WHERE namespace.nspname = 'public' AND constraint_definition.conname = ?`, name).Scan(&count).Error; err != nil {
			t.Fatalf("read constraint %s: %v", name, err)
		}
		if count != 1 {
			t.Fatalf("constraint %s count = %d, want 1", name, count)
		}
	}
}

func assertPostgresConstraintError(t *testing.T, err error, constraintName string) {
	t.Helper()
	if err == nil {
		t.Fatalf("PostgreSQL accepted a value guarded by %s", constraintName)
	}
	if !strings.Contains(err.Error(), constraintName) {
		t.Fatalf("constraint error = %v, want %s", err, constraintName)
	}
}

func postgresRoundTripAuditRun(t *testing.T) *agentaudit.Run {
	t.Helper()
	run := validAgentAuditCreateRun(t)

	startedAt := time.Now()
	remainder := time.Duration(startedAt.Nanosecond()%1000) * time.Nanosecond
	startedAt = startedAt.Add(400*time.Nanosecond - remainder)
	finishedAt := startedAt.Add(time.Millisecond - 200*time.Nanosecond)
	run.StartedAt = startedAt
	run.FinishedAt = finishedAt
	run.DurationMS = finishedAt.Sub(startedAt).Milliseconds()
	run.CreatedAt = finishedAt.Add(time.Second)

	agentStartedAt, agentFinishedAt := startedAt, finishedAt
	agentDurationMS := agentFinishedAt.Sub(agentStartedAt).Milliseconds()
	run.AgentStartedAt = &agentStartedAt
	run.AgentFinishedAt = &agentFinishedAt
	run.AgentDurationMS = &agentDurationMS

	usageJSON := `{ "reported_model_calls" : 0, "missing_model_calls" : 0, "reported_totals" : { } }`
	modelCallsJSON := `[ ]`
	run.UsageJSON = &usageJSON
	run.ModelCallsJSON = &modelCallsJSON

	firstArguments := `{ "query_length" : 3 }`
	firstResult := `{ "source_count" : 0, "source_ids" : [ ], "match_modes" : [ ] }`
	secondArguments := `{"query_length":6}`
	secondResult := `{"match_modes":[],"source_ids":[],"source_count":0}`
	run.ToolCalls[0].StartedAt = startedAt.Add(250*time.Microsecond + 300*time.Nanosecond)
	run.ToolCalls[0].ArgumentsJSON = firstArguments
	run.ToolCalls[0].ArgumentsOriginalBytes = int64(len(firstArguments) + 11)
	run.ToolCalls[0].ArgumentsCapturedBytes = int64(len(firstArguments))
	run.ToolCalls[0].ResultJSON = firstResult
	run.ToolCalls[0].ResultOriginalBytes = int64(len(firstResult) + 13)
	run.ToolCalls[0].ResultCapturedBytes = int64(len(firstResult))

	second := run.ToolCalls[0]
	second.Sequence = 2
	second.StartedAt = startedAt.Add(500*time.Microsecond + 200*time.Nanosecond)
	second.ArgumentsJSON = secondArguments
	second.ArgumentsOriginalBytes = int64(len(secondArguments) + 17)
	second.ArgumentsCapturedBytes = int64(len(secondArguments))
	second.ResultJSON = secondResult
	second.ResultOriginalBytes = int64(len(secondResult) + 19)
	second.ResultCapturedBytes = int64(len(secondResult))
	run.ToolCalls = append(run.ToolCalls, second)
	*run.ToolCallCount = len(run.ToolCalls)

	if err := run.ValidateForCreate(); err != nil {
		t.Fatalf("PostgreSQL round-trip fixture is invalid: %v", err)
	}
	return run
}

func assertPostgresAuditPayloads(t *testing.T, run *agentaudit.Run, wantUsageJSON, wantModelCallsJSON string, wantPayloads [][2]string) {
	t.Helper()
	if run.UsageJSON == nil || *run.UsageJSON != wantUsageJSON || run.ModelCallsJSON == nil || *run.ModelCallsJSON != wantModelCallsJSON {
		t.Fatalf("run JSON was not preserved: usage=%v models=%v", run.UsageJSON, run.ModelCallsJSON)
	}
	if len(run.ToolCalls) != len(wantPayloads) {
		t.Fatalf("tool call count = %d, want %d", len(run.ToolCalls), len(wantPayloads))
	}
	for i := range run.ToolCalls {
		call := &run.ToolCalls[i]
		if call.Sequence != int64(i+1) || call.ID == 0 || call.RunID != run.ID {
			t.Fatalf("tool[%d] persistence identity/order = id %d run %d sequence %d", i, call.ID, call.RunID, call.Sequence)
		}
		if call.ArgumentsJSON != wantPayloads[i][0] || call.ResultJSON != wantPayloads[i][1] {
			t.Fatalf("tool[%d] JSON was not preserved: arguments=%q result=%q", i, call.ArgumentsJSON, call.ResultJSON)
		}
		if call.ArgumentsCapturedBytes != int64(len(call.ArgumentsJSON)) || call.ResultCapturedBytes != int64(len(call.ResultJSON)) {
			t.Fatalf("tool[%d] captured bytes = arguments %d/%d, result %d/%d", i, call.ArgumentsCapturedBytes, len(call.ArgumentsJSON), call.ResultCapturedBytes, len(call.ResultJSON))
		}
		if call.StartedAt.Nanosecond()%1000 != 0 || call.CreatedAt.Nanosecond()%1000 != 0 {
			t.Fatalf("tool[%d] did not return PostgreSQL-normalized timestamps: start=%v created=%v", i, call.StartedAt, call.CreatedAt)
		}
	}
}

func cloneAgentAuditRunForPostgresTest(source *agentaudit.Run) *agentaudit.Run {
	result := *source
	result.UserID = cloneAgentAuditTestPointer(source.UserID)
	result.ConversationID = cloneAgentAuditTestPointer(source.ConversationID)
	result.SessionID = cloneAgentAuditTestPointer(source.SessionID)
	result.ActorRole = cloneAgentAuditTestPointer(source.ActorRole)
	result.HTTPStatus = cloneAgentAuditTestPointer(source.HTTPStatus)
	result.FailureSource = cloneAgentAuditTestPointer(source.FailureSource)
	result.FailureCategory = cloneAgentAuditTestPointer(source.FailureCategory)
	result.FailureCode = cloneAgentAuditTestPointer(source.FailureCode)
	result.FailureRetryable = cloneAgentAuditTestPointer(source.FailureRetryable)
	result.AgentStartedAt = cloneAgentAuditTestPointer(source.AgentStartedAt)
	result.AgentFinishedAt = cloneAgentAuditTestPointer(source.AgentFinishedAt)
	result.AgentDurationMS = cloneAgentAuditTestPointer(source.AgentDurationMS)
	result.ConversationPersisted = cloneAgentAuditTestPointer(source.ConversationPersisted)
	result.ModelCallCount = cloneAgentAuditTestPointer(source.ModelCallCount)
	result.ToolCallCount = cloneAgentAuditTestPointer(source.ToolCallCount)
	result.DroppedModelCalls = cloneAgentAuditTestPointer(source.DroppedModelCalls)
	result.DroppedToolCalls = cloneAgentAuditTestPointer(source.DroppedToolCalls)
	result.ReportedModelCalls = cloneAgentAuditTestPointer(source.ReportedModelCalls)
	result.MissingModelCalls = cloneAgentAuditTestPointer(source.MissingModelCalls)
	result.PromptTokens = cloneAgentAuditTestPointer(source.PromptTokens)
	result.CompletionTokens = cloneAgentAuditTestPointer(source.CompletionTokens)
	result.TotalTokens = cloneAgentAuditTestPointer(source.TotalTokens)
	result.CachedTokens = cloneAgentAuditTestPointer(source.CachedTokens)
	result.ReasoningTokens = cloneAgentAuditTestPointer(source.ReasoningTokens)
	result.UsageJSON = cloneAgentAuditTestPointer(source.UsageJSON)
	result.ModelCallsJSON = cloneAgentAuditTestPointer(source.ModelCallsJSON)
	result.ToolCalls = append([]agentaudit.ToolCall(nil), source.ToolCalls...)
	for i := range result.ToolCalls {
		result.ToolCalls[i].CallID = cloneAgentAuditTestPointer(source.ToolCalls[i].CallID)
		result.ToolCalls[i].ErrorCategory = cloneAgentAuditTestPointer(source.ToolCalls[i].ErrorCategory)
		result.ToolCalls[i].ErrorCode = cloneAgentAuditTestPointer(source.ToolCalls[i].ErrorCode)
		result.ToolCalls[i].ErrorType = cloneAgentAuditTestPointer(source.ToolCalls[i].ErrorType)
		result.ToolCalls[i].ErrorHTTPStatus = cloneAgentAuditTestPointer(source.ToolCalls[i].ErrorHTTPStatus)
		result.ToolCalls[i].ErrorRetryable = cloneAgentAuditTestPointer(source.ToolCalls[i].ErrorRetryable)
		result.ToolCalls[i].BusinessObjectType = cloneAgentAuditTestPointer(source.ToolCalls[i].BusinessObjectType)
		result.ToolCalls[i].BusinessObjectID = cloneAgentAuditTestPointer(source.ToolCalls[i].BusinessObjectID)
		result.ToolCalls[i].BusinessObjectStatus = cloneAgentAuditTestPointer(source.ToolCalls[i].BusinessObjectStatus)
	}
	return &result
}

func cloneAgentAuditTestPointer[T any](source *T) *T {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func randomAgentAuditRequestID(t *testing.T) string {
	t.Helper()
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		t.Fatalf("generate unique request ID: %v", err)
	}
	return hex.EncodeToString(value[:])
}
