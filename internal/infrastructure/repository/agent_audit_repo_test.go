package repository

import (
	"context"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"SWPUCAT/internal/domain/agentaudit"
	"SWPUCAT/internal/infrastructure/database"

	"github.com/DATA-DOG/go-sqlmock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAgentAuditRepoCreateRunReturnsCompleteWinnerBeforeOwnershipLookup(t *testing.T) {
	repo, mock := newAgentAuditMockRepo(t)
	_, winnerModel, toolModels := persistedAgentAuditWinner(t)
	jsonbNormalizedUsage := `{"missing_model_calls": 0, "reported_model_calls": 0, "reported_totals": {}}`
	winnerModel.UsageJSON = &jsonbNormalizedUsage
	winner := agentAuditRunFromModels(&winnerModel, toolModels)
	expectAgentAuditWinner(mock, &winnerModel, toolModels)

	candidate := validAgentAuditCreateRun(t)
	userID := int64(404)
	conversationID := int64(405)
	sessionID := "deleted-conversation"
	candidate.UserID = &userID
	candidate.ConversationID = &conversationID
	candidate.SessionID = &sessionID
	if err := repo.CreateRun(context.Background(), candidate); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if !reflect.DeepEqual(candidate, winner) {
		t.Fatalf("CreateRun() did not replace candidate with winner\n got: %#v\nwant: %#v", candidate, winner)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAuditRepoCreateRunEnforcesPersistenceContractBeforeDatabaseAccess(t *testing.T) {
	repo, mock := newAgentAuditMockRepo(t)
	run := validAgentAuditCreateRun(t)
	unsafeUsage := `{"reported_model_calls":0,"missing_model_calls":0,"reported_totals":{},"private_payload":"secret"}`
	run.UsageJSON = &unsafeUsage

	if err := repo.CreateRun(context.Background(), run); !errors.Is(err, agentaudit.ErrInvalidRun) {
		t.Fatalf("CreateRun() error = %v, want ErrInvalidRun", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAuditRepoCreateRunAcceptsExactPersistedAggregateRetry(t *testing.T) {
	repo, mock := newAgentAuditMockRepo(t)
	winner, winnerModel, toolModels := persistedAgentAuditWinner(t)
	expectAgentAuditWinner(mock, &winnerModel, toolModels)

	retry := agentAuditRunFromModels(&winnerModel, toolModels)
	if err := repo.CreateRun(context.Background(), retry); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if !reflect.DeepEqual(retry, winner) {
		t.Fatalf("retry result mismatch\n got: %#v\nwant: %#v", retry, winner)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAuditRepoCreateRunAllowsFirstSuccessThenSameObjectRetry(t *testing.T) {
	repo, mock := newAgentAuditMockRepo(t)
	run := validAgentAuditCreateRun(t)
	winnerModel := agentRunModelFromDomain(run)
	winnerModel.ID = 91
	winnerModel.StartedAt = winnerModel.StartedAt.Round(time.Microsecond)
	winnerModel.FinishedAt = winnerModel.FinishedAt.Round(time.Microsecond)
	winnerModel.CreatedAt = winnerModel.CreatedAt.Round(time.Microsecond)
	agentStartedAt := winnerModel.AgentStartedAt.Round(time.Microsecond)
	agentFinishedAt := winnerModel.AgentFinishedAt.Round(time.Microsecond)
	winnerModel.AgentStartedAt = &agentStartedAt
	winnerModel.AgentFinishedAt = &agentFinishedAt
	winnerTool := agentToolCallModelFromDomain(winnerModel.ID, run.CreatedAt, &run.ToolCalls[0])
	winnerTool.ID = 201
	winnerTool.StartedAt = winnerTool.StartedAt.Round(time.Microsecond)
	winnerTool.CreatedAt = winnerTool.CreatedAt.Round(time.Microsecond)

	mock.ExpectBegin()
	expectAgentAuditRequestLock(mock, run.RequestID)
	mock.ExpectQuery(agentRunLookupPattern()).
		WithArgs(run.RequestID, 1).
		WillReturnRows(sqlmock.NewRows(agentRunColumns()))
	mock.ExpectQuery(`INSERT INTO "agent_runs"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(winnerModel.ID))
	mock.ExpectQuery(`INSERT INTO "agent_tool_calls"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(winnerTool.ID))
	expectAgentAuditWinnerQueries(mock, &winnerModel, []database.AgentToolCallModel{winnerTool})
	mock.ExpectCommit()
	expectAgentAuditWinner(mock, &winnerModel, []database.AgentToolCallModel{winnerTool})

	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("first CreateRun() error = %v", err)
	}
	want := agentAuditRunFromModels(&winnerModel, []database.AgentToolCallModel{winnerTool})
	if !reflect.DeepEqual(run, want) {
		t.Fatalf("first CreateRun() did not return database winner\n got: %#v\nwant: %#v", run, want)
	}
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("retry CreateRun() error = %v", err)
	}
	if !reflect.DeepEqual(run, want) {
		t.Fatalf("retry result mismatch\n got: %#v\nwant: %#v", run, want)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAuditRepoCreateRunLoadsConcurrentConflictWinner(t *testing.T) {
	repo, mock := newAgentAuditMockRepo(t)
	run := validAgentAuditCreateRun(t)
	winner, winnerModel, toolModels := persistedAgentAuditWinner(t)

	mock.ExpectBegin()
	expectAgentAuditRequestLock(mock, run.RequestID)
	mock.ExpectQuery(agentRunLookupPattern()).
		WithArgs(run.RequestID, 1).
		WillReturnRows(sqlmock.NewRows(agentRunColumns()))
	mock.ExpectQuery(`INSERT INTO "agent_runs"`).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	expectAgentAuditWinnerQueries(mock, &winnerModel, toolModels)
	mock.ExpectCommit()

	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("CreateRun() error = %v", err)
	}
	if !reflect.DeepEqual(run, winner) {
		t.Fatalf("concurrent winner mismatch\n got: %#v\nwant: %#v", run, winner)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentAuditRepoCreateRunRejectsForgedPersistenceIdentities(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*agentaudit.Run)
		wantErr error
		queries bool
	}{
		{
			name: "run id",
			mutate: func(run *agentaudit.Run) {
				run.ID++
				for i := range run.ToolCalls {
					run.ToolCalls[i].RunID = run.ID
				}
			},
			wantErr: agentaudit.ErrInvalidRun,
			queries: true,
		},
		{
			name: "tool id",
			mutate: func(run *agentaudit.Run) {
				run.ToolCalls[0].ID++
			},
			wantErr: agentaudit.ErrInvalidToolCall,
			queries: true,
		},
		{
			name: "tool run id",
			mutate: func(run *agentaudit.Run) {
				run.ToolCalls[0].RunID++
			},
			wantErr: agentaudit.ErrInvalidToolCall,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo, mock := newAgentAuditMockRepo(t)
			winner, winnerModel, toolModels := persistedAgentAuditWinner(t)
			test.mutate(winner)
			if test.queries {
				expectAgentAuditWinnerRollback(mock, &winnerModel, toolModels)
			}
			if err := repo.CreateRun(context.Background(), winner); !errors.Is(err, test.wantErr) {
				t.Fatalf("CreateRun() error = %v, want %v", err, test.wantErr)
			}
			if err := mock.ExpectationsWereMet(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestAgentAuditRepoCreateRunRejectsPreassignedIdentityWhenNoWinnerExists(t *testing.T) {
	repo, mock := newAgentAuditMockRepo(t)
	candidate := validAgentAuditCreateRun(t)
	candidate.ID = 91
	candidate.ToolCalls[0].ID = 201
	candidate.ToolCalls[0].RunID = 91

	mock.ExpectBegin()
	expectAgentAuditRequestLock(mock, candidate.RequestID)
	mock.ExpectQuery(agentRunLookupPattern()).
		WithArgs(candidate.RequestID, 1).
		WillReturnRows(sqlmock.NewRows(agentRunColumns()))
	mock.ExpectRollback()
	if err := repo.CreateRun(context.Background(), candidate); !errors.Is(err, agentaudit.ErrInvalidRun) {
		t.Fatalf("CreateRun() error = %v, want ErrInvalidRun", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAgentRunModelFromDomainMapsEveryField(t *testing.T) {
	startedAt := time.Date(2026, 9, 18, 2, 3, 4, 5, time.UTC)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	agentStartedAt := startedAt.Add(100 * time.Millisecond)
	agentFinishedAt := startedAt.Add(1200 * time.Millisecond)
	createdAt := finishedAt.Add(time.Second)
	userID := int64(7)
	conversationID := int64(11)
	sessionID := "session-7"
	actorRole := "member"
	httpStatus := 503
	failureSource := "model"
	failureCategory := "availability"
	failureCode := "upstream_unavailable"
	failureRetryable := true
	agentDurationMS := int64(1100)
	conversationPersisted := false
	modelCallCount := 3
	toolCallCount := 2
	droppedModelCalls := 1
	droppedToolCalls := 1
	reportedModelCalls := 1
	missingModelCalls := 2
	promptTokens := int64(17)
	completionTokens := int64(8)
	totalTokens := int64(25)
	cachedTokens := int64(4)
	reasoningTokens := int64(3)
	usageJSON := `{"reported_model_calls":1}`
	modelCallsJSON := `[{"sequence":1}]`

	run := &agentaudit.Run{
		ID: 41, RequestID: "0123456789abcdef0123456789abcdef",
		UserID: &userID, ConversationID: &conversationID, SessionID: &sessionID, ActorRole: &actorRole,
		Status: agentaudit.RunStatusUnavailable, TelemetryStatus: agentaudit.TelemetryCaptured,
		HTTPStatus: &httpStatus, FailureSource: &failureSource, FailureCategory: &failureCategory,
		FailureCode: &failureCode, FailureRetryable: &failureRetryable,
		StartedAt: startedAt, FinishedAt: finishedAt, DurationMS: 1500,
		AgentStartedAt: &agentStartedAt, AgentFinishedAt: &agentFinishedAt, AgentDurationMS: &agentDurationMS,
		ConversationPersisted: &conversationPersisted,
		ModelCallCount:        &modelCallCount, ToolCallCount: &toolCallCount,
		DroppedModelCalls: &droppedModelCalls, DroppedToolCalls: &droppedToolCalls,
		ReportedModelCalls: &reportedModelCalls, MissingModelCalls: &missingModelCalls,
		PromptTokens: &promptTokens, CompletionTokens: &completionTokens, TotalTokens: &totalTokens,
		CachedTokens: &cachedTokens, ReasoningTokens: &reasoningTokens,
		UsageJSON: &usageJSON, ModelCallsJSON: &modelCallsJSON, CreatedAt: createdAt,
	}

	want := database.AgentRunModel{
		ID: 41, RequestID: run.RequestID,
		UserID: &userID, ConversationID: &conversationID, SessionID: &sessionID, ActorRole: &actorRole,
		Status: string(agentaudit.RunStatusUnavailable), TelemetryStatus: string(agentaudit.TelemetryCaptured),
		HTTPStatus: &httpStatus, FailureSource: &failureSource, FailureCategory: &failureCategory,
		FailureCode: &failureCode, FailureRetryable: &failureRetryable,
		StartedAt: startedAt, FinishedAt: finishedAt, DurationMS: 1500,
		AgentStartedAt: &agentStartedAt, AgentFinishedAt: &agentFinishedAt, AgentDurationMS: &agentDurationMS,
		ConversationPersisted: &conversationPersisted,
		ModelCallCount:        &modelCallCount, ToolCallCount: &toolCallCount,
		DroppedModelCalls: &droppedModelCalls, DroppedToolCalls: &droppedToolCalls,
		UsageReportedModelCalls: &reportedModelCalls, UsageMissingModelCalls: &missingModelCalls,
		PromptTokens: &promptTokens, CompletionTokens: &completionTokens, TotalTokens: &totalTokens,
		CachedTokens: &cachedTokens, ReasoningTokens: &reasoningTokens,
		UsageJSON: &usageJSON, ModelCallsJSON: &modelCallsJSON, CreatedAt: createdAt,
	}
	if got := agentRunModelFromDomain(run); !reflect.DeepEqual(got, want) {
		t.Fatalf("agentRunModelFromDomain() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAgentToolCallModelFromDomainMapsEveryField(t *testing.T) {
	startedAt := time.Date(2026, 9, 18, 2, 3, 4, 5, time.UTC)
	createdAt := startedAt.Add(time.Second)
	callID := "call-9"
	errorCategory := "execution"
	errorCode := "tool_failed"
	errorType := "upstream"
	errorHTTPStatus := 502
	errorRetryable := true
	businessObjectType := "announcement_draft"
	businessObjectID := int64(73)
	businessObjectStatus := "draft"
	call := &agentaudit.ToolCall{
		ID: 51, RunID: 41, Sequence: 2, CallID: &callID, Name: "draft_announcement",
		IsWrite: true, Status: string(agentaudit.ToolCallStatusError), StartedAt: startedAt, DurationMS: 23,
		ArgumentsJSON: `{"title_length":4}`, ArgumentsOriginalBytes: 100,
		ArgumentsCapturedBytes: 18, ArgumentsRedacted: true, ArgumentsTruncated: true,
		ResultJSON: `{"status":"failed"}`, ResultOriginalBytes: 200,
		ResultCapturedBytes: 19, ResultRedacted: true, ResultTruncated: false,
		ErrorCategory: &errorCategory, ErrorCode: &errorCode, ErrorType: &errorType,
		ErrorHTTPStatus: &errorHTTPStatus, ErrorRetryable: &errorRetryable,
		BusinessObjectType: &businessObjectType, BusinessObjectID: &businessObjectID,
		BusinessObjectStatus: &businessObjectStatus, CreatedAt: createdAt,
	}

	want := database.AgentToolCallModel{
		ID: 51, RunID: 99, Sequence: 2, CallID: &callID, ToolName: "draft_announcement",
		IsWrite: true, Status: string(agentaudit.ToolCallStatusError), StartedAt: startedAt, DurationMS: 23,
		ArgumentsJSON: `{"title_length":4}`, ArgumentsOriginalBytes: 100,
		ArgumentsCapturedBytes: 18, ArgumentsRedacted: true, ArgumentsTruncated: true,
		ResultJSON: `{"status":"failed"}`, ResultOriginalBytes: 200,
		ResultCapturedBytes: 19, ResultRedacted: true, ResultTruncated: false,
		ErrorCategory: &errorCategory, ErrorCode: &errorCode, ErrorType: &errorType,
		ErrorHTTPStatus: &errorHTTPStatus, ErrorRetryable: &errorRetryable,
		BusinessObjectType: &businessObjectType, BusinessObjectID: &businessObjectID,
		BusinessObjectStatus: &businessObjectStatus, CreatedAt: createdAt,
	}
	if got := agentToolCallModelFromDomain(99, time.Time{}, call); !reflect.DeepEqual(got, want) {
		t.Fatalf("agentToolCallModelFromDomain() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAgentAuditRunFromModelsMapsEveryField(t *testing.T) {
	startedAt := time.Date(2026, 9, 18, 2, 3, 4, 5, time.UTC)
	finishedAt := startedAt.Add(1500 * time.Millisecond)
	agentStartedAt := startedAt.Add(100 * time.Millisecond)
	agentFinishedAt := startedAt.Add(1200 * time.Millisecond)
	createdAt := finishedAt.Add(time.Second)
	toolCreatedAt := createdAt.Add(time.Millisecond)
	userID := int64(7)
	conversationID := int64(11)
	sessionID := "session-7"
	actorRole := "member"
	httpStatus := 503
	failureSource := "model"
	failureCategory := "availability"
	failureCode := "upstream_unavailable"
	failureRetryable := true
	agentDurationMS := int64(1100)
	conversationPersisted := false
	modelCallCount := 3
	toolCallCount := 2
	droppedModelCalls := 1
	droppedToolCalls := 1
	reportedModelCalls := 1
	missingModelCalls := 2
	promptTokens := int64(17)
	completionTokens := int64(8)
	totalTokens := int64(25)
	cachedTokens := int64(4)
	reasoningTokens := int64(3)
	usageJSON := `{"reported_model_calls":1}`
	modelCallsJSON := `[{"sequence":1}]`
	callID := "call-9"
	errorCategory := "execution"
	errorCode := "tool_failed"
	errorType := "upstream"
	errorHTTPStatus := 502
	errorRetryable := true
	businessObjectType := "announcement_draft"
	businessObjectID := int64(73)
	businessObjectStatus := "draft"

	runModel := database.AgentRunModel{
		ID: 41, RequestID: "0123456789abcdef0123456789abcdef",
		UserID: &userID, ConversationID: &conversationID, SessionID: &sessionID, ActorRole: &actorRole,
		Status: string(agentaudit.RunStatusUnavailable), TelemetryStatus: string(agentaudit.TelemetryCaptured),
		HTTPStatus: &httpStatus, FailureSource: &failureSource, FailureCategory: &failureCategory,
		FailureCode: &failureCode, FailureRetryable: &failureRetryable,
		StartedAt: startedAt, FinishedAt: finishedAt, DurationMS: 1500,
		AgentStartedAt: &agentStartedAt, AgentFinishedAt: &agentFinishedAt, AgentDurationMS: &agentDurationMS,
		ConversationPersisted: &conversationPersisted,
		ModelCallCount:        &modelCallCount, ToolCallCount: &toolCallCount,
		DroppedModelCalls: &droppedModelCalls, DroppedToolCalls: &droppedToolCalls,
		UsageReportedModelCalls: &reportedModelCalls, UsageMissingModelCalls: &missingModelCalls,
		PromptTokens: &promptTokens, CompletionTokens: &completionTokens, TotalTokens: &totalTokens,
		CachedTokens: &cachedTokens, ReasoningTokens: &reasoningTokens,
		UsageJSON: &usageJSON, ModelCallsJSON: &modelCallsJSON, CreatedAt: createdAt,
	}
	toolModel := database.AgentToolCallModel{
		ID: 51, RunID: 41, Sequence: 2, CallID: &callID, ToolName: "draft_announcement",
		IsWrite: true, Status: string(agentaudit.ToolCallStatusError), StartedAt: agentStartedAt, DurationMS: 23,
		ArgumentsJSON: `{"title_length":4}`, ArgumentsOriginalBytes: 100,
		ArgumentsCapturedBytes: 18, ArgumentsRedacted: true, ArgumentsTruncated: true,
		ResultJSON: `{"status":"failed"}`, ResultOriginalBytes: 200,
		ResultCapturedBytes: 19, ResultRedacted: true, ResultTruncated: false,
		ErrorCategory: &errorCategory, ErrorCode: &errorCode, ErrorType: &errorType,
		ErrorHTTPStatus: &errorHTTPStatus, ErrorRetryable: &errorRetryable,
		BusinessObjectType: &businessObjectType, BusinessObjectID: &businessObjectID,
		BusinessObjectStatus: &businessObjectStatus, CreatedAt: toolCreatedAt,
	}
	want := &agentaudit.Run{
		ID: 41, RequestID: runModel.RequestID,
		UserID: &userID, ConversationID: &conversationID, SessionID: &sessionID, ActorRole: &actorRole,
		Status: agentaudit.RunStatusUnavailable, TelemetryStatus: agentaudit.TelemetryCaptured,
		HTTPStatus: &httpStatus, FailureSource: &failureSource, FailureCategory: &failureCategory,
		FailureCode: &failureCode, FailureRetryable: &failureRetryable,
		StartedAt: startedAt, FinishedAt: finishedAt, DurationMS: 1500,
		AgentStartedAt: &agentStartedAt, AgentFinishedAt: &agentFinishedAt, AgentDurationMS: &agentDurationMS,
		ConversationPersisted: &conversationPersisted,
		ModelCallCount:        &modelCallCount, ToolCallCount: &toolCallCount,
		DroppedModelCalls: &droppedModelCalls, DroppedToolCalls: &droppedToolCalls,
		ReportedModelCalls: &reportedModelCalls, MissingModelCalls: &missingModelCalls,
		PromptTokens: &promptTokens, CompletionTokens: &completionTokens, TotalTokens: &totalTokens,
		CachedTokens: &cachedTokens, ReasoningTokens: &reasoningTokens,
		UsageJSON: &usageJSON, ModelCallsJSON: &modelCallsJSON,
		ToolCalls: []agentaudit.ToolCall{{
			ID: 51, RunID: 41, Sequence: 2, CallID: &callID, Name: "draft_announcement",
			IsWrite: true, Status: string(agentaudit.ToolCallStatusError), StartedAt: agentStartedAt, DurationMS: 23,
			ArgumentsJSON: `{"title_length":4}`, ArgumentsOriginalBytes: 100,
			ArgumentsCapturedBytes: 18, ArgumentsRedacted: true, ArgumentsTruncated: true,
			ResultJSON: `{"status":"failed"}`, ResultOriginalBytes: 200,
			ResultCapturedBytes: 19, ResultRedacted: true, ResultTruncated: false,
			ErrorCategory: &errorCategory, ErrorCode: &errorCode, ErrorType: &errorType,
			ErrorHTTPStatus: &errorHTTPStatus, ErrorRetryable: &errorRetryable,
			BusinessObjectType: &businessObjectType, BusinessObjectID: &businessObjectID,
			BusinessObjectStatus: &businessObjectStatus, CreatedAt: toolCreatedAt,
		}},
		CreatedAt: createdAt,
	}

	if got := agentAuditRunFromModels(&runModel, []database.AgentToolCallModel{toolModel}); !reflect.DeepEqual(got, want) {
		t.Fatalf("agentAuditRunFromModels() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestAgentToolCallModelUsesRunCreationTimeFallback(t *testing.T) {
	fallback := time.Date(2026, 9, 18, 2, 3, 4, 5, time.UTC)
	call := &agentaudit.ToolCall{}
	if got := agentToolCallModelFromDomain(3, fallback, call).CreatedAt; !got.Equal(fallback) {
		t.Fatalf("CreatedAt = %v, want %v", got, fallback)
	}
}

func TestAuditModelsHaveNoFreeFormMessageOrParamFields(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(database.AgentRunModel{}), reflect.TypeOf(database.AgentToolCallModel{})} {
		for _, field := range []string{"Message", "Param", "ErrorMessage", "ErrorParam", "FailureMessage"} {
			if _, exists := typ.FieldByName(field); exists {
				t.Fatalf("%s unexpectedly persists %s", typ.Name(), field)
			}
		}
	}
}

func validAgentAuditCreateRun(t *testing.T) *agentaudit.Run {
	t.Helper()
	startedAt := time.Date(2026, 9, 18, 2, 3, 4, 5, time.UTC)
	run, err := agentaudit.NewRun(agentaudit.RunMetadata{
		RequestID:       strings.Repeat("a", 32),
		Status:          agentaudit.RunStatusSuccess,
		TelemetryStatus: agentaudit.TelemetryStatusCaptured,
		StartedAt:       startedAt,
		FinishedAt:      startedAt.Add(time.Second),
		CreatedAt:       startedAt.Add(2 * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	agentStartedAt := startedAt.Add(100 * time.Millisecond)
	agentFinishedAt := startedAt.Add(900 * time.Millisecond)
	agentDurationMS := agentFinishedAt.Sub(agentStartedAt).Milliseconds()
	modelCallCount := 0
	toolCallCount := 1
	droppedModelCalls := 0
	droppedToolCalls := 0
	reportedModelCalls := 0
	missingModelCalls := 0
	usageJSON := `{"reported_model_calls":0,"missing_model_calls":0,"reported_totals":{}}`
	modelCallsJSON := `[]`
	run.AgentStartedAt = &agentStartedAt
	run.AgentFinishedAt = &agentFinishedAt
	run.AgentDurationMS = &agentDurationMS
	run.ModelCallCount = &modelCallCount
	run.ToolCallCount = &toolCallCount
	run.DroppedModelCalls = &droppedModelCalls
	run.DroppedToolCalls = &droppedToolCalls
	run.ReportedModelCalls = &reportedModelCalls
	run.MissingModelCalls = &missingModelCalls
	run.UsageJSON = &usageJSON
	run.ModelCallsJSON = &modelCallsJSON
	run.ToolCalls = []agentaudit.ToolCall{{
		Sequence:               1,
		Name:                   "search_knowledge",
		Status:                 string(agentaudit.ToolCallStatusSuccess),
		StartedAt:              agentStartedAt,
		ArgumentsJSON:          `{}`,
		ArgumentsOriginalBytes: 2,
		ArgumentsCapturedBytes: 2,
		ResultJSON:             `{}`,
		ResultOriginalBytes:    2,
		ResultCapturedBytes:    2,
	}}
	if err := run.ValidateForCreate(); err != nil {
		t.Fatalf("test fixture invalid: %v", err)
	}
	return run
}

func persistedAgentAuditWinner(t *testing.T) (*agentaudit.Run, database.AgentRunModel, []database.AgentToolCallModel) {
	t.Helper()
	winner := validAgentAuditCreateRun(t)
	winner.ID = 91
	winner.ToolCalls[0].ID = 201
	winner.ToolCalls[0].RunID = winner.ID
	winner.ToolCalls[0].CreatedAt = winner.CreatedAt.Add(time.Millisecond)
	second := winner.ToolCalls[0]
	second.ID = 202
	second.Sequence = 2
	second.StartedAt = second.StartedAt.Add(time.Millisecond)
	second.CreatedAt = second.CreatedAt.Add(time.Millisecond)
	winner.ToolCalls = append(winner.ToolCalls, second)
	*winner.ToolCallCount = 2

	runModel := agentRunModelFromDomain(winner)
	toolModels := make([]database.AgentToolCallModel, len(winner.ToolCalls))
	for i := range winner.ToolCalls {
		toolModels[i] = agentToolCallModelFromDomain(winner.ID, winner.CreatedAt, &winner.ToolCalls[i])
	}
	return agentAuditRunFromModels(&runModel, toolModels), runModel, toolModels
}

func newAgentAuditMockRepo(t *testing.T) (*AgentAuditRepo, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB, PreferSimpleProtocol: true}), &gorm.Config{
		DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	return NewAgentAuditRepo(db), mock
}

func expectAgentAuditWinner(mock sqlmock.Sqlmock, run *database.AgentRunModel, tools []database.AgentToolCallModel) {
	mock.ExpectBegin()
	expectAgentAuditRequestLock(mock, run.RequestID)
	expectAgentAuditWinnerQueries(mock, run, tools)
	mock.ExpectCommit()
}

func expectAgentAuditWinnerRollback(mock sqlmock.Sqlmock, run *database.AgentRunModel, tools []database.AgentToolCallModel) {
	mock.ExpectBegin()
	expectAgentAuditRequestLock(mock, run.RequestID)
	expectAgentAuditWinnerQueries(mock, run, tools)
	mock.ExpectRollback()
}

func expectAgentAuditRequestLock(mock sqlmock.Sqlmock, requestID string) {
	mock.ExpectExec(`SELECT pg_advisory_xact_lock\(hashtextextended\(\$1, 0\)\)`).
		WithArgs(requestID).
		WillReturnResult(sqlmock.NewResult(0, 1))
}

func expectAgentAuditWinnerQueries(mock sqlmock.Sqlmock, run *database.AgentRunModel, tools []database.AgentToolCallModel) {
	mock.ExpectQuery(agentRunLookupPattern()).
		WithArgs(run.RequestID, 1).
		WillReturnRows(agentRunRows(run))
	mock.ExpectQuery(agentToolLookupPattern()).
		WithArgs(run.ID).
		WillReturnRows(agentToolRows(tools))
}

func agentRunLookupPattern() string {
	return `SELECT .* FROM "agent_runs" WHERE request_id = \$1 LIMIT \$2`
}

func agentToolLookupPattern() string {
	return `SELECT .* FROM "agent_tool_calls" WHERE run_id = \$1 ORDER BY sequence ASC`
}

func agentRunColumns() []string {
	return []string{
		"id", "request_id", "user_id", "conversation_id", "session_id", "actor_role",
		"status", "telemetry_status", "http_status", "failure_source", "failure_category", "failure_code", "failure_retryable",
		"started_at", "finished_at", "duration_ms", "agent_started_at", "agent_finished_at", "agent_duration_ms",
		"conversation_persisted", "model_call_count", "tool_call_count", "dropped_model_calls", "dropped_tool_calls",
		"usage_reported_model_calls", "usage_missing_model_calls", "prompt_tokens", "completion_tokens", "total_tokens",
		"cached_tokens", "reasoning_tokens", "usage_json", "model_calls_json", "created_at",
	}
}

func agentRunRows(run *database.AgentRunModel) *sqlmock.Rows {
	return sqlmock.NewRows(agentRunColumns()).AddRow(
		run.ID, run.RequestID, int64SQLValue(run.UserID), int64SQLValue(run.ConversationID), stringSQLValue(run.SessionID), stringSQLValue(run.ActorRole),
		run.Status, run.TelemetryStatus, intSQLValue(run.HTTPStatus), stringSQLValue(run.FailureSource), stringSQLValue(run.FailureCategory), stringSQLValue(run.FailureCode), boolSQLValue(run.FailureRetryable),
		run.StartedAt, run.FinishedAt, run.DurationMS, timeSQLValue(run.AgentStartedAt), timeSQLValue(run.AgentFinishedAt), int64SQLValue(run.AgentDurationMS),
		boolSQLValue(run.ConversationPersisted), intSQLValue(run.ModelCallCount), intSQLValue(run.ToolCallCount), intSQLValue(run.DroppedModelCalls), intSQLValue(run.DroppedToolCalls),
		intSQLValue(run.UsageReportedModelCalls), intSQLValue(run.UsageMissingModelCalls), int64SQLValue(run.PromptTokens), int64SQLValue(run.CompletionTokens), int64SQLValue(run.TotalTokens),
		int64SQLValue(run.CachedTokens), int64SQLValue(run.ReasoningTokens), stringSQLValue(run.UsageJSON), stringSQLValue(run.ModelCallsJSON), run.CreatedAt,
	)
}

func agentToolRows(tools []database.AgentToolCallModel) *sqlmock.Rows {
	rows := sqlmock.NewRows([]string{
		"id", "run_id", "sequence", "call_id", "tool_name", "is_write", "status", "started_at", "duration_ms",
		"arguments_json", "arguments_original_bytes", "arguments_captured_bytes", "arguments_redacted", "arguments_truncated",
		"result_json", "result_original_bytes", "result_captured_bytes", "result_redacted", "result_truncated",
		"error_category", "error_code", "error_type", "error_http_status", "error_retryable",
		"business_object_type", "business_object_id", "business_object_status", "created_at",
	})
	for i := range tools {
		call := &tools[i]
		rows.AddRow(
			call.ID, call.RunID, call.Sequence, stringSQLValue(call.CallID), call.ToolName, call.IsWrite, call.Status, call.StartedAt, call.DurationMS,
			call.ArgumentsJSON, call.ArgumentsOriginalBytes, call.ArgumentsCapturedBytes, call.ArgumentsRedacted, call.ArgumentsTruncated,
			call.ResultJSON, call.ResultOriginalBytes, call.ResultCapturedBytes, call.ResultRedacted, call.ResultTruncated,
			stringSQLValue(call.ErrorCategory), stringSQLValue(call.ErrorCode), stringSQLValue(call.ErrorType), intSQLValue(call.ErrorHTTPStatus), boolSQLValue(call.ErrorRetryable),
			stringSQLValue(call.BusinessObjectType), int64SQLValue(call.BusinessObjectID), stringSQLValue(call.BusinessObjectStatus), call.CreatedAt,
		)
	}
	return rows
}

func intSQLValue(value *int) driver.Value {
	if value == nil {
		return nil
	}
	return int64(*value)
}

func int64SQLValue(value *int64) driver.Value {
	if value == nil {
		return nil
	}
	return *value
}

func stringSQLValue(value *string) driver.Value {
	if value == nil {
		return nil
	}
	return *value
}

func boolSQLValue(value *bool) driver.Value {
	if value == nil {
		return nil
	}
	return *value
}

func timeSQLValue(value *time.Time) driver.Value {
	if value == nil {
		return nil
	}
	return *value
}
