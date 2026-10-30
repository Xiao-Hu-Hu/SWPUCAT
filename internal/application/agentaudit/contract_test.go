package agentauditapp

import (
	"context"
	"strings"
	"testing"

	"SWPUCAT/internal/agent/telemetry"
	"SWPUCAT/internal/domain/agentaudit"
)

func TestValidatePersistenceContractAcceptsServiceProjections(t *testing.T) {
	tests := []struct {
		name string
		req  RecordRequest
	}{
		{name: "not started", req: validRecordRequest(agentaudit.TelemetryNotStarted)},
		{name: "unavailable", req: validRecordRequest(agentaudit.TelemetryUnavailable)},
		{name: "invalid", req: validRecordRequest(agentaudit.TelemetryInvalid)},
		{name: "captured", req: func() RecordRequest {
			req := validRecordRequest(agentaudit.TelemetryCaptured)
			req.Snapshot = capturedSnapshot()
			return req
		}()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), test.req)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidatePersistenceContract(run); err != nil {
				t.Fatalf("service projection rejected: %v", err)
			}
		})
	}
}

func TestValidatePersistenceContractRejectsPrivacyBypasses(t *testing.T) {
	validRun := func(t *testing.T) *agentaudit.Run {
		t.Helper()
		req := validRecordRequest(agentaudit.TelemetryCaptured)
		req.Snapshot = capturedSnapshot()
		run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return run
	}

	tests := []struct {
		name   string
		mutate func(*agentaudit.Run)
	}{
		{
			name: "usage unknown prompt field",
			mutate: func(run *agentaudit.Run) {
				value := `{"prompt":"SECRET_USAGE_913"}`
				run.UsageJSON = &value
			},
		},
		{
			name: "model unknown answer field",
			mutate: func(run *agentaudit.Run) {
				value := `[{"answer":"SECRET_MODEL_913"}]`
				run.ModelCallsJSON = &value
			},
		},
		{
			name: "known tool extra raw field",
			mutate: func(run *agentaudit.Run) {
				value := `{"content":"SECRET_TOOL_913","title_length":4}`
				run.ToolCalls[0].ArgumentsJSON = value
				run.ToolCalls[0].ArgumentsCapturedBytes = int64(len(value))
			},
		},
		{
			name: "canonical token mismatch",
			mutate: func(run *agentaudit.Run) {
				value := int64(999)
				run.PromptTokens = &value
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := validRun(t)
			test.mutate(run)
			if err := run.Validate(); err != nil {
				t.Fatalf("fixture must demonstrate a domain-level bypass, got %v", err)
			}
			if err := ValidatePersistenceContract(run); err == nil {
				t.Fatal("strict persistence contract accepted unsafe projection")
			}
		})
	}
}

func TestValidatePersistenceContractRejectsUnknownToolRawPayload(t *testing.T) {
	snapshot := capturedSnapshot()
	snapshot.ToolCalls[0].Name = "future_unknown_tool"
	snapshot.ModelCalls[0].ToolRequests[0].Name = "future_unknown_tool"
	snapshot.ToolCalls[0].Arguments.JSON = `{"prompt":"SECRET_UNKNOWN_ARGUMENT_913"}`
	snapshot.ToolCalls[0].Result.JSON = `{"answer":"SECRET_UNKNOWN_RESULT_913"}`
	snapshot.ToolCalls[0].Arguments.CapturedBytes = len(snapshot.ToolCalls[0].Arguments.JSON)
	snapshot.ToolCalls[0].Result.CapturedBytes = len(snapshot.ToolCalls[0].Result.JSON)
	req := validRecordRequest(agentaudit.TelemetryCaptured)
	req.Snapshot = snapshot
	run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if run.ToolCalls[0].Name != "[invalid]" || !strings.Contains(run.ToolCalls[0].ArgumentsJSON, `"type":"object"`) {
		t.Fatalf("unexpected safe unknown-tool projection: %#v", run.ToolCalls[0])
	}

	raw := `{"prompt":"SECRET_BYPASS_913"}`
	run.ToolCalls[0].ArgumentsJSON = raw
	run.ToolCalls[0].ArgumentsCapturedBytes = int64(len(raw))
	if err := run.Validate(); err != nil {
		t.Fatalf("fixture must demonstrate a domain-level bypass, got %v", err)
	}
	if err := ValidatePersistenceContract(run); err == nil {
		t.Fatal("unknown tool raw payload bypassed shape contract")
	}
}

func TestValidatePersistenceContractRejectsUnproducibleUnknownToolProjection(t *testing.T) {
	validRun := func(t *testing.T) *agentaudit.Run {
		t.Helper()
		snapshot := capturedSnapshot()
		snapshot.ToolCalls[0].Name = "future_unknown_tool"
		snapshot.ModelCalls[0].ToolRequests[0].Name = "future_unknown_tool"
		req := validRecordRequest(agentaudit.TelemetryCaptured)
		req.Snapshot = snapshot
		run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		return run
	}

	tests := []struct {
		name   string
		mutate func(*agentaudit.ToolCall)
	}{
		{
			name: "arguments not marked redacted",
			mutate: func(call *agentaudit.ToolCall) {
				call.ArgumentsRedacted = false
			},
		},
		{
			name: "result not marked redacted",
			mutate: func(call *agentaudit.ToolCall) {
				call.ResultRedacted = false
			},
		},
		{
			name: "unproducible unknown scalar type",
			mutate: func(call *agentaudit.ToolCall) {
				call.ArgumentsJSON = `{"type":"unknown"}`
				call.ArgumentsCapturedBytes = int64(len(call.ArgumentsJSON))
			},
		},
		{
			name: "unproducible depth marker",
			mutate: func(call *agentaudit.ToolCall) {
				call.ArgumentsJSON = `{"type":"depth_limited"}`
				call.ArgumentsCapturedBytes = int64(len(call.ArgumentsJSON))
			},
		},
		{
			name: "empty array with item type",
			mutate: func(call *agentaudit.ToolCall) {
				call.ArgumentsJSON = `{"count":0,"item_types":["string"],"type":"array"}`
				call.ArgumentsCapturedBytes = int64(len(call.ArgumentsJSON))
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := validRun(t)
			test.mutate(&run.ToolCalls[0])
			if err := run.Validate(); err != nil {
				t.Fatalf("fixture must demonstrate a domain-level bypass, got %v", err)
			}
			if err := ValidatePersistenceContract(run); err == nil {
				t.Fatal("strict persistence contract accepted a projection Service cannot emit")
			}
		})
	}
}

func TestValidatePersistenceContractRejectsOutOfOrderToolCalls(t *testing.T) {
	snapshot := capturedSnapshot()
	second := snapshot.ToolCalls[0]
	second.Sequence = 3
	second.CallID = "call-2"
	snapshot.ToolCalls = append(snapshot.ToolCalls, second)
	snapshot.ToolCallCount = 2
	snapshot.ModelCalls[0].ToolRequests = append(snapshot.ModelCalls[0].ToolRequests, telemetry.ToolRequest{
		CallID: "call-2",
		Name:   "draft_announcement",
	})
	req := validRecordRequest(agentaudit.TelemetryCaptured)
	req.Snapshot = snapshot
	run, err := NewService(&fakeAuditRepo{}).Record(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	run.ToolCalls[0], run.ToolCalls[1] = run.ToolCalls[1], run.ToolCalls[0]
	if err := run.Validate(); err != nil {
		t.Fatalf("fixture must demonstrate a domain-level bypass, got %v", err)
	}
	if err := ValidatePersistenceContract(run); err == nil {
		t.Fatal("strict persistence contract accepted out-of-order tool calls")
	}
}
