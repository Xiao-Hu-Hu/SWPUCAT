package agentauditapp

import (
	"context"
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"SWPUCAT/internal/agent/telemetry"
	"SWPUCAT/internal/domain/agentaudit"
)

const auditTestRequestID = "0123456789abcdef0123456789abcdef"

type fakeAuditRepo struct {
	run *agentaudit.Run
	err error
}

func (r *fakeAuditRepo) CreateRun(_ context.Context, run *agentaudit.Run) error {
	if r.err != nil {
		return r.err
	}
	r.run = run
	if run.ID == 0 {
		run.ID = 91
	}
	return nil
}

func validRecordRequest(status agentaudit.TelemetryStatus) RecordRequest {
	started := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	userID := int64(7)
	sessionID := "session-7"
	role := "member"
	httpStatus := 200
	return RecordRequest{
		RequestID:       auditTestRequestID,
		UserID:          &userID,
		SessionID:       &sessionID,
		ActorRole:       &role,
		Status:          agentaudit.RunStatusSuccess,
		HTTPStatus:      &httpStatus,
		StartedAt:       started,
		FinishedAt:      started.Add(1500 * time.Millisecond),
		TelemetryStatus: status,
	}
}

func capturedSnapshot() *telemetry.Snapshot {
	started := time.Date(2026, 9, 18, 9, 0, 0, 100_000_000, time.UTC)
	usage := telemetry.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15}
	arguments := `{"title_length":4,"content_length":20,"PRIVATE_PROMPT":"SECRET_PAYLOAD_913"}`
	result := `{"id":17,"status":"draft","pinned":false,"content":"SECRET_RESULT_913"}`
	return &telemetry.Snapshot{
		StartedAt:      started,
		FinishedAt:     started.Add(900 * time.Millisecond),
		DurationMS:     900,
		ModelCallCount: 1,
		ToolCallCount:  1,
		Usage: telemetry.UsageSummary{
			ReportedModelCalls: 1,
			ReportedTotals:     usage,
			CompleteTotals:     &usage,
		},
		ModelCalls: []telemetry.ModelCall{{
			Sequence: 1, Provider: "openai_compatible", Model: "test-model", Status: "success",
			StartedAt: started, DurationMS: 400, InputMessages: 2, ToolsOffered: 3,
			FinishReason: "tool_calls", ProviderResponseAlias: "response-1", Usage: &usage,
			ToolRequests: []telemetry.ToolRequest{{CallID: "call-1", Name: "draft_announcement"}},
		}},
		ToolCalls: []telemetry.ToolCall{{
			Sequence: 2, CallID: "call-1", Name: "draft_announcement", Status: "success",
			StartedAt: started.Add(500 * time.Millisecond), DurationMS: 100,
			Arguments: telemetry.PayloadCapture{JSON: arguments, OriginalBytes: 2048, CapturedBytes: len(arguments), Redacted: true},
			Result:    telemetry.PayloadCapture{JSON: result, OriginalBytes: 4096, CapturedBytes: len(result), Redacted: true},
		}},
	}
}

func TestRecordProjectsCapturedSnapshotWithoutPrivateContent(t *testing.T) {
	repo := &fakeAuditRepo{}
	service := NewService(repo)
	req := validRecordRequest(agentaudit.TelemetryCaptured)
	req.Snapshot = capturedSnapshot()

	run, err := service.Record(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if repo.run != run || run.TelemetryStatus != agentaudit.TelemetryCaptured || run.ID != 91 {
		t.Fatalf("unexpected persisted run: %#v", run)
	}
	if run.ModelCallCount == nil || *run.ModelCallCount != 1 || run.ToolCallCount == nil || *run.ToolCallCount != 1 {
		t.Fatalf("captured counts = model %#v tool %#v", run.ModelCallCount, run.ToolCallCount)
	}
	if run.PromptTokens == nil || *run.PromptTokens != 10 || run.CompletionTokens == nil || *run.CompletionTokens != 5 || run.TotalTokens == nil || *run.TotalTokens != 15 {
		t.Fatalf("canonical tokens = prompt %#v completion %#v total %#v", run.PromptTokens, run.CompletionTokens, run.TotalTokens)
	}
	if run.CachedTokens != nil || run.ReasoningTokens != nil {
		t.Fatalf("ambiguous zero token details were persisted: cached %#v reasoning %#v", run.CachedTokens, run.ReasoningTokens)
	}
	if strings.Contains(*run.UsageJSON, "cached_tokens") || strings.Contains(*run.UsageJSON, "reasoning_tokens") {
		t.Fatalf("ambiguous token details leaked into usage projection: %s", *run.UsageJSON)
	}
	if len(run.ToolCalls) != 1 || !run.ToolCalls[0].IsWrite || run.ToolCalls[0].BusinessObjectID == nil || *run.ToolCalls[0].BusinessObjectID != 17 {
		t.Fatalf("tool projection = %#v", run.ToolCalls)
	}
	serialized := *run.UsageJSON + *run.ModelCallsJSON + run.ToolCalls[0].ArgumentsJSON + run.ToolCalls[0].ResultJSON
	for _, secret := range []string{"SECRET_PAYLOAD_913", "SECRET_RESULT_913", "PRIVATE_PROMPT", `"content"`} {
		if strings.Contains(serialized, secret) {
			t.Fatalf("persisted audit leaked %q: %s", secret, serialized)
		}
	}
}

func TestRecordPreservesUnknownVersusKnownZeroTelemetry(t *testing.T) {
	tests := []struct {
		name          string
		status        agentaudit.TelemetryStatus
		snapshot      *telemetry.Snapshot
		wantStatus    agentaudit.TelemetryStatus
		wantKnownZero bool
	}{
		{name: "not started", status: agentaudit.TelemetryNotStarted, wantStatus: agentaudit.TelemetryNotStarted, wantKnownZero: true},
		{name: "unavailable", status: agentaudit.TelemetryUnavailable, wantStatus: agentaudit.TelemetryUnavailable},
		{name: "invalid", status: agentaudit.TelemetryInvalid, wantStatus: agentaudit.TelemetryInvalid},
		{name: "captured zero", status: agentaudit.TelemetryCaptured, snapshot: zeroSnapshot(), wantStatus: agentaudit.TelemetryCaptured, wantKnownZero: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeAuditRepo{}
			req := validRecordRequest(test.status)
			req.Snapshot = test.snapshot
			run, err := NewService(repo).Record(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if run.TelemetryStatus != test.wantStatus {
				t.Fatalf("telemetry status = %q, want %q", run.TelemetryStatus, test.wantStatus)
			}
			if test.wantKnownZero {
				if run.ModelCallCount == nil || *run.ModelCallCount != 0 || run.ToolCallCount == nil || *run.ToolCallCount != 0 || run.ModelCallsJSON == nil || *run.ModelCallsJSON != "[]" {
					t.Fatalf("known zero was not preserved: %#v", run)
				}
			} else if run.ModelCallCount != nil || run.ToolCallCount != nil || run.UsageJSON != nil || run.ModelCallsJSON != nil {
				t.Fatalf("unknown telemetry was written as known: %#v", run)
			}
			if test.status == agentaudit.TelemetryNotStarted && run.AgentStartedAt != nil || test.status == agentaudit.TelemetryUnavailable && run.AgentStartedAt != nil {
				t.Fatalf("non-executed telemetry has Agent timing: %#v", run.AgentStartedAt)
			}
		})
	}
}

func TestRecordPartialUsageKeepsCanonicalTokensNullAndDropsErrorText(t *testing.T) {
	snapshot := capturedSnapshot()
	snapshot.ModelCallCount = 2
	snapshot.Usage.ReportedModelCalls = 1
	snapshot.Usage.MissingModelCalls = 1
	snapshot.Usage.CompleteTotals = nil
	snapshot.ModelCalls = append(snapshot.ModelCalls, telemetry.ModelCall{
		Sequence: 3, Provider: "openai_compatible", Model: "test-model", Status: "error",
		StartedAt: snapshot.StartedAt.Add(700 * time.Millisecond), DurationMS: 50, InputMessages: 3, ToolsOffered: 3,
		Error: &telemetry.ErrorInfo{Category: "execution", Code: "execution_error", Param: "SECRET_PARAM_913", Message: "SECRET_MESSAGE_913"},
	})
	req := validRecordRequest(agentaudit.TelemetryCaptured)
	req.Snapshot = snapshot
	run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if run.PromptTokens != nil || run.CompletionTokens != nil || run.TotalTokens != nil || run.CachedTokens != nil || run.ReasoningTokens != nil {
		t.Fatalf("partial usage became canonical totals: %#v", run)
	}
	for _, secret := range []string{"SECRET_PARAM_913", "SECRET_MESSAGE_913", `"message"`, `"param"`} {
		if strings.Contains(*run.ModelCallsJSON, secret) {
			t.Fatalf("model projection leaked %q: %s", secret, *run.ModelCallsJSON)
		}
	}
}

func TestRecordOnlyPersistsTokenDimensionReportedByEveryModelCall(t *testing.T) {
	snapshot := capturedSnapshot()
	first := telemetry.TokenUsage{PromptTokens: 10, CompletionTokens: 5, TotalTokens: 15, CachedTokens: 2}
	second := telemetry.TokenUsage{PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10, CachedTokens: 4, ReasoningTokens: 6}
	totals := telemetry.TokenUsage{PromptTokens: 17, CompletionTokens: 8, TotalTokens: 25, CachedTokens: 6, ReasoningTokens: 6}
	snapshot.ModelCalls[0].Usage = &first
	snapshot.ModelCalls = append(snapshot.ModelCalls, telemetry.ModelCall{
		Sequence: 3, Provider: "openai_compatible", Model: "test-model", Status: "success",
		StartedAt: snapshot.StartedAt.Add(700 * time.Millisecond), DurationMS: 50, InputMessages: 3, ToolsOffered: 3,
		Usage: &second,
	})
	snapshot.ModelCallCount = 2
	snapshot.Usage.ReportedModelCalls = 2
	snapshot.Usage.ReportedTotals = totals
	snapshot.Usage.CompleteTotals = &totals

	req := validRecordRequest(agentaudit.TelemetryCaptured)
	req.Snapshot = snapshot
	run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if run.CachedTokens == nil || *run.CachedTokens != 6 {
		t.Fatalf("complete cached tokens = %#v, want 6", run.CachedTokens)
	}
	if run.ReasoningTokens != nil {
		t.Fatalf("partially reported reasoning tokens = %#v, want nil", run.ReasoningTokens)
	}
	if strings.Contains(*run.UsageJSON, `"reasoning_tokens"`) {
		t.Fatalf("partial reasoning total was projected as complete: %s", *run.UsageJSON)
	}
}

func TestRecordAcceptsDetailOnlyTokenUsage(t *testing.T) {
	tests := []struct {
		name      string
		usage     telemetry.TokenUsage
		wantField func(*agentaudit.Run) *int64
		wantValue int64
	}{
		{
			name:      "cached only",
			usage:     telemetry.TokenUsage{CachedTokens: 6},
			wantField: func(run *agentaudit.Run) *int64 { return run.CachedTokens },
			wantValue: 6,
		},
		{
			name:      "reasoning only",
			usage:     telemetry.TokenUsage{ReasoningTokens: 4},
			wantField: func(run *agentaudit.Run) *int64 { return run.ReasoningTokens },
			wantValue: 4,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := capturedSnapshot()
			snapshot.ModelCalls[0].Usage = &test.usage
			snapshot.Usage.ReportedTotals = test.usage
			snapshot.Usage.CompleteTotals = &test.usage

			req := validRecordRequest(agentaudit.TelemetryCaptured)
			req.Snapshot = snapshot
			run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			field := test.wantField(run)
			if field == nil || *field != test.wantValue || run.TelemetryStatus != agentaudit.TelemetryCaptured {
				t.Fatalf("detail-only usage was not preserved: %#v", run)
			}
		})
	}
}

func TestRecordAllowsProviderCallAliasReuseAcrossModelRounds(t *testing.T) {
	snapshot := capturedSnapshot()
	usage := *snapshot.ModelCalls[0].Usage
	snapshot.ModelCalls = append(snapshot.ModelCalls, telemetry.ModelCall{
		Sequence: 3, Provider: "openai_compatible", Model: "test-model", Status: "success",
		StartedAt: snapshot.StartedAt.Add(650 * time.Millisecond), DurationMS: 50, InputMessages: 3, ToolsOffered: 3,
		FinishReason: "tool_calls", ProviderResponseAlias: "response-2", Usage: &usage,
		ToolRequests: []telemetry.ToolRequest{{CallID: "call-1", Name: "draft_announcement"}},
	})
	secondTool := snapshot.ToolCalls[0]
	secondTool.Sequence = 4
	secondTool.StartedAt = snapshot.StartedAt.Add(750 * time.Millisecond)
	snapshot.ToolCalls = append(snapshot.ToolCalls, secondTool)
	snapshot.ModelCallCount = 2
	snapshot.ToolCallCount = 2
	totals := telemetry.TokenUsage{PromptTokens: 20, CompletionTokens: 10, TotalTokens: 30}
	snapshot.Usage.ReportedModelCalls = 2
	snapshot.Usage.ReportedTotals = totals
	snapshot.Usage.CompleteTotals = &totals

	req := validRecordRequest(agentaudit.TelemetryCaptured)
	req.Snapshot = snapshot
	run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if run.TelemetryStatus != agentaudit.TelemetryCaptured || len(run.ToolCalls) != 2 {
		t.Fatalf("reused provider call alias invalidated telemetry: %#v", run)
	}
}

func TestRecordDowngradesInvalidSnapshotButKeepsRun(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*telemetry.Snapshot)
	}{
		{name: "duplicate sequence", mutate: func(snapshot *telemetry.Snapshot) { snapshot.ToolCalls[0].Sequence = 1 }},
		{name: "non-terminal model snapshot", mutate: func(snapshot *telemetry.Snapshot) {
			snapshot.ModelCallCount++
			snapshot.Usage.MissingModelCalls++
			snapshot.Usage.CompleteTotals = nil
		}},
		{name: "non-terminal tool snapshot", mutate: func(snapshot *telemetry.Snapshot) { snapshot.ToolCallCount++ }},
		{name: "invalid json", mutate: func(snapshot *telemetry.Snapshot) { snapshot.ToolCalls[0].Result.JSON = `{"id":` }},
		{name: "trailing json", mutate: func(snapshot *telemetry.Snapshot) {
			snapshot.ToolCalls[0].Result.JSON = `{} {}`
			snapshot.ToolCalls[0].Result.CapturedBytes = len(snapshot.ToolCalls[0].Result.JSON)
		}},
		{name: "capture size mismatch", mutate: func(snapshot *telemetry.Snapshot) { snapshot.ToolCalls[0].Result.CapturedBytes++ }},
		{name: "negative duration", mutate: func(snapshot *telemetry.Snapshot) { snapshot.ModelCalls[0].DurationMS = -1 }},
		{name: "snapshot duration too large", mutate: func(snapshot *telemetry.Snapshot) { snapshot.DurationMS = math.MaxInt64 }},
		{name: "tool duration too large", mutate: func(snapshot *telemetry.Snapshot) { snapshot.ToolCalls[0].DurationMS = math.MaxInt64 }},
		{name: "negative token", mutate: func(snapshot *telemetry.Snapshot) { snapshot.ModelCalls[0].Usage.PromptTokens = -1 }},
		{name: "invalid status", mutate: func(snapshot *telemetry.Snapshot) { snapshot.ToolCalls[0].Status = "invented" }},
		{name: "missing complete totals", mutate: func(snapshot *telemetry.Snapshot) { snapshot.Usage.CompleteTotals = nil }},
		{name: "unexpected complete totals", mutate: func(snapshot *telemetry.Snapshot) {
			snapshot.ModelCalls[0].Usage = nil
			snapshot.Usage.ReportedModelCalls = 0
			snapshot.Usage.MissingModelCalls = 1
			snapshot.Usage.ReportedTotals = telemetry.TokenUsage{}
		}},
		{name: "empty error metadata", mutate: func(snapshot *telemetry.Snapshot) {
			snapshot.ModelCalls[0].Status = "error"
			snapshot.ModelCalls[0].Usage = nil
			snapshot.ModelCalls[0].Error = &telemetry.ErrorInfo{}
			snapshot.Usage.ReportedModelCalls = 0
			snapshot.Usage.MissingModelCalls = 1
			snapshot.Usage.ReportedTotals = telemetry.TokenUsage{}
			snapshot.Usage.CompleteTotals = nil
		}},
		{name: "unsafe error metadata", mutate: func(snapshot *telemetry.Snapshot) {
			snapshot.ModelCalls[0].Status = "error"
			snapshot.ModelCalls[0].Usage = nil
			snapshot.ModelCalls[0].Error = &telemetry.ErrorInfo{Category: "execution", Code: "sk-secret"}
			snapshot.Usage.ReportedModelCalls = 0
			snapshot.Usage.MissingModelCalls = 1
			snapshot.Usage.ReportedTotals = telemetry.TokenUsage{}
			snapshot.Usage.CompleteTotals = nil
		}},
		{name: "status error category mismatch", mutate: func(snapshot *telemetry.Snapshot) {
			snapshot.ModelCalls[0].Status = "panic"
			snapshot.ModelCalls[0].Usage = nil
			snapshot.ModelCalls[0].Error = &telemetry.ErrorInfo{Category: "execution", Code: "execution_error"}
			snapshot.Usage.ReportedModelCalls = 0
			snapshot.Usage.MissingModelCalls = 1
			snapshot.Usage.ReportedTotals = telemetry.TokenUsage{}
			snapshot.Usage.CompleteTotals = nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := capturedSnapshot()
			test.mutate(snapshot)
			repo := &fakeAuditRepo{}
			req := validRecordRequest(agentaudit.TelemetryCaptured)
			req.Snapshot = snapshot
			run, err := NewService(repo).Record(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if repo.run == nil || run.TelemetryStatus != agentaudit.TelemetryInvalid || run.ModelCallCount != nil || len(run.ToolCalls) != 0 {
				t.Fatalf("invalid snapshot was partially trusted: %#v", run)
			}
		})
	}
}

func TestRecordShapesUnknownToolPayloadAndPropagatesRepositoryFailure(t *testing.T) {
	snapshot := capturedSnapshot()
	snapshot.ToolCalls[0].Name = "future_unknown_tool"
	snapshot.ModelCalls[0].ToolRequests[0].Name = "future_unknown_tool"
	snapshot.ToolCalls[0].Arguments.JSON = `{"prompt":"SECRET_UNKNOWN_ARGUMENT_913"}`
	snapshot.ToolCalls[0].Result.JSON = `{"answer":"SECRET_UNKNOWN_RESULT_913"}`
	snapshot.ToolCalls[0].Arguments.CapturedBytes = len(snapshot.ToolCalls[0].Arguments.JSON)
	snapshot.ToolCalls[0].Result.CapturedBytes = len(snapshot.ToolCalls[0].Result.JSON)
	snapshot.ToolCalls[0].Arguments.Redacted = false
	snapshot.ToolCalls[0].Result.Redacted = false
	req := validRecordRequest(agentaudit.TelemetryCaptured)
	req.Snapshot = snapshot
	run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	projection := run.ToolCalls[0].ArgumentsJSON + run.ToolCalls[0].ResultJSON
	if strings.Contains(projection, "SECRET_UNKNOWN") || !strings.Contains(projection, `"type":"object"`) || !run.ToolCalls[0].IsWrite || !run.ToolCalls[0].ArgumentsRedacted || !run.ToolCalls[0].ResultRedacted {
		t.Fatalf("unknown payload projection = %s", projection)
	}

	wantErr := errors.New("database unavailable")
	_, err = NewService(&fakeAuditRepo{err: wantErr}).Record(context.Background(), validRecordRequest(agentaudit.TelemetryNotStarted))
	if !errors.Is(err, wantErr) {
		t.Fatalf("repository error = %v, want %v", err, wantErr)
	}
}

func TestRecordRejectsInvalidTrustedMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RecordRequest)
	}{
		{name: "request id", mutate: func(req *RecordRequest) { req.RequestID = "not-trusted" }},
		{name: "main timing", mutate: func(req *RecordRequest) { req.FinishedAt = req.StartedAt.Add(-time.Second) }},
		{name: "failure code", mutate: func(req *RecordRequest) { req.Failure = &Failure{Source: "agent", Code: "SECRET CODE"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := &fakeAuditRepo{}
			req := validRecordRequest(agentaudit.TelemetryNotStarted)
			test.mutate(&req)
			if _, err := NewService(repo).Record(context.Background(), req); err == nil {
				t.Fatal("accepted invalid trusted metadata")
			}
			if repo.run != nil {
				t.Fatal("invalid metadata reached repository")
			}
		})
	}
}

func TestProjectPayloadRetainsSafePeriodAndStatusCounts(t *testing.T) {
	raw := `{"period_id":17,"status_counts":{"normal":3,"attention":1,"id":13800138000},"private_note":"SECRET_NOTE_913"}`
	projected, redacted, _, err := projectPayload("get_checkin_stats", raw, maxResultProjectionBytes, true, persistedPayloadResult)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(projected, "period_id") || strings.Contains(projected, `"id"`) ||
		!strings.Contains(projected, `"normal":3`) || !strings.Contains(projected, `"attention":1`) ||
		strings.Contains(projected, "SECRET_NOTE_913") || !redacted {
		t.Fatalf("safe payload projection = %s, redacted=%v", projected, redacted)
	}
}

func TestProjectPayloadSeparatesToolAndDirectionSchemas(t *testing.T) {
	tests := []struct {
		name     string
		tool     string
		kind     persistedPayloadKind
		raw      string
		contains []string
		excludes []string
	}{
		{
			name: "result fields cannot enter arguments", tool: "search_knowledge", kind: persistedPayloadArguments,
			raw:      `{"query_length":12,"source_ids":[72,101,108,108,111]}`,
			contains: []string{`"query_length":12`}, excludes: []string{"source_ids", "72"},
		},
		{
			name: "other tool fields are rejected", tool: "get_checkin_status", kind: persistedPayloadResult,
			raw:      `{"status":"idle","announcement_id":13800138000}`,
			contains: []string{`"status":"idle"`}, excludes: []string{"announcement_id", "13800138000"},
		},
		{
			name: "dynamic report states retain only enum counts", tool: "query_team_members", kind: persistedPayloadResult,
			raw:      `{"report_states":{"missing":2,"submitted":1,"source_ids":17}}`,
			contains: []string{`"missing":2`, `"submitted":1`}, excludes: []string{"source_ids", "17"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projected, redacted, _, err := projectPayload(test.tool, test.raw, maxResultProjectionBytes, true, test.kind)
			if err != nil {
				t.Fatal(err)
			}
			for _, value := range test.contains {
				if !strings.Contains(projected, value) {
					t.Fatalf("projection %s does not contain %q", projected, value)
				}
			}
			for _, value := range test.excludes {
				if strings.Contains(projected, value) {
					t.Fatalf("projection %s contains forbidden %q", projected, value)
				}
			}
			if !redacted {
				t.Fatalf("schema reduction was not marked redacted: %s", projected)
			}
		})
	}
}

func TestProjectPayloadRejectsOversubscribedDynamicCounts(t *testing.T) {
	raw := `{"status_counts":{"normal":15,"attention":10}}`
	projected, redacted, _, err := projectPayload("get_checkin_stats", raw, maxResultProjectionBytes, true, persistedPayloadResult)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(projected, `"status_counts":{}`) || !redacted {
		t.Fatalf("oversubscribed count projection = %s, redacted=%v", projected, redacted)
	}
}

func zeroSnapshot() *telemetry.Snapshot {
	started := time.Date(2026, 9, 18, 9, 0, 0, 0, time.UTC)
	return &telemetry.Snapshot{
		StartedAt:  started,
		FinishedAt: started.Add(time.Millisecond),
		DurationMS: 1,
		Usage:      telemetry.UsageSummary{},
		ModelCalls: []telemetry.ModelCall{},
		ToolCalls:  []telemetry.ToolCall{},
	}
}
