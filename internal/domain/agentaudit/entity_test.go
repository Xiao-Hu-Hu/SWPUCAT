package agentaudit

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNewRunValidatesIdentityOwnershipAndMachineFailure(t *testing.T) {
	base := validRunMetadata()
	if _, err := NewRun(base); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		mutate func(*RunMetadata)
	}{
		{name: "forged request id", mutate: func(input *RunMetadata) { input.RequestID = strings.Repeat("A", 32) }},
		{name: "session without user", mutate: func(input *RunMetadata) { input.UserID = nil }},
		{name: "conversation without session", mutate: func(input *RunMetadata) { input.SessionID = nil }},
		{name: "unknown actor role", mutate: func(input *RunMetadata) { input.ActorRole = stringPtr("administrator") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := validRunMetadata()
			test.mutate(&input)
			if _, err := NewRun(input); !errors.Is(err, ErrInvalidRun) {
				t.Fatalf("NewRun() error = %v, want ErrInvalidRun", err)
			}
		})
	}

	deletedConversation := validRunMetadata()
	deletedConversation.ConversationID = nil
	historicalRun, err := NewRun(deletedConversation)
	if err != nil {
		t.Fatalf("historical conversation persistence rejected after association deletion: %v", err)
	}
	if err := historicalRun.ValidateForCreate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("new run persisted state without conversation error = %v, want ErrInvalidRun", err)
	}

	failureInput := validRunMetadata()
	failureInput.Status = RunStatusUnavailable
	failureInput.FailureSource = stringPtr("network")
	failureInput.FailureCategory = stringPtr("connection")
	failureInput.FailureCode = stringPtr("refused")
	failureInput.FailureRetryable = boolPtr(true)
	if _, err := NewRun(failureInput); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("unknown failure source error = %v, want ErrInvalidRun", err)
	}
	failureInput.FailureSource = stringPtr("transport")
	if _, err := NewRun(failureInput); err != nil {
		t.Fatalf("valid machine-readable failure rejected: %v", err)
	}
}

func TestRunTelemetryDistinguishesKnownZeroFromUnavailable(t *testing.T) {
	run, err := NewRun(validRunMetadata())
	if err != nil {
		t.Fatal(err)
	}
	zero := 0
	usage := `{"reported_model_calls":0,"missing_model_calls":0,"reported_totals":{}}`
	models := `[]`
	run.TelemetryStatus = TelemetryNotStarted
	run.ModelCallCount = &zero
	run.ToolCallCount = intPtr(0)
	run.DroppedModelCalls = intPtr(0)
	run.DroppedToolCalls = intPtr(0)
	run.ReportedModelCalls = intPtr(0)
	run.MissingModelCalls = intPtr(0)
	run.UsageJSON = &usage
	run.ModelCallsJSON = &models
	if err := run.Validate(); err != nil {
		t.Fatalf("known zero telemetry rejected: %v", err)
	}
	run.PromptTokens = int64Ptr(1)
	if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("not_started token error = %v, want ErrInvalidRun", err)
	}
	run.PromptTokens = nil

	run.UsageJSON = nil
	if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("not_started without known usage error = %v, want ErrInvalidRun", err)
	}

	run.TelemetryStatus = TelemetryUnavailable
	run.ModelCallCount = nil
	run.ToolCallCount = nil
	run.DroppedModelCalls = nil
	run.DroppedToolCalls = nil
	run.ReportedModelCalls = nil
	run.MissingModelCalls = nil
	run.ModelCallsJSON = nil
	if err := run.Validate(); err != nil {
		t.Fatalf("unavailable telemetry rejected: %v", err)
	}
	run.ModelCallCount = intPtr(0)
	if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("unavailable zero count error = %v, want ErrInvalidRun", err)
	}
}

func TestCapturedRunAllowsIndependentPositiveTokensAndBoundsToolProjection(t *testing.T) {
	run := validCapturedRun(t)
	if err := run.Validate(); err != nil {
		t.Fatalf("valid captured run rejected: %v", err)
	}

	run.ReasoningTokens = nil
	if err := run.Validate(); err != nil {
		t.Fatalf("independently unknown reasoning tokens rejected: %v", err)
	}
	run.CompletionTokens = nil
	if err := run.Validate(); err != nil {
		t.Fatalf("independently unknown completion tokens rejected: %v", err)
	}
	run.CompletionTokens = int64Ptr(0)
	if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("ambiguous zero canonical token error = %v, want ErrInvalidRun", err)
	}
	run.CompletionTokens = int64Ptr(5)
	run.ReasoningTokens = int64Ptr(-1)
	if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("negative canonical token error = %v, want ErrInvalidRun", err)
	}
	run.ReasoningTokens = nil

	run.ToolCalls[0].BusinessObjectID = int64Ptr(0)
	if err := run.Validate(); !errors.Is(err, ErrInvalidToolCall) {
		t.Fatalf("invalid business object id error = %v, want ErrInvalidToolCall", err)
	}
	run.ToolCalls[0].BusinessObjectID = int64Ptr(7)

	run.ToolCalls[0].ArgumentsCapturedBytes++
	if err := run.Validate(); !errors.Is(err, ErrInvalidToolCall) {
		t.Fatalf("mismatched captured bytes error = %v, want ErrInvalidToolCall", err)
	}
}

func TestCapturedRunEnforcesPersistenceBoundsBeforeDatabaseWrite(t *testing.T) {
	t.Run("telemetry count", func(t *testing.T) {
		run := validCapturedRun(t)
		run.ToolCallCount = intPtr(MaxTelemetryCount + 1)
		run.DroppedToolCalls = intPtr(MaxTelemetryCount)
		if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
			t.Fatalf("oversized telemetry count error = %v, want ErrInvalidRun", err)
		}
	})

	t.Run("model projection rows", func(t *testing.T) {
		run := validCapturedRun(t)
		models := "[" + strings.TrimSuffix(strings.Repeat("{},", MaxPersistedModelCalls+1), ",") + "]"
		run.ModelCallsJSON = &models
		run.ModelCallCount = intPtr(MaxPersistedModelCalls + 1)
		run.ReportedModelCalls = intPtr(MaxPersistedModelCalls + 1)
		if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
			t.Fatalf("oversized model projection error = %v, want ErrInvalidRun", err)
		}
	})

	t.Run("tool duration", func(t *testing.T) {
		run := validCapturedRun(t)
		run.ToolCalls[0].DurationMS = MaxCallDurationMS + 1
		if err := run.Validate(); !errors.Is(err, ErrInvalidToolCall) {
			t.Fatalf("oversized tool duration error = %v, want ErrInvalidToolCall", err)
		}
	})
}

func TestReadOnlyToolRejectsBusinessObjectProjection(t *testing.T) {
	run := validCapturedRun(t)
	run.ToolCalls[0].IsWrite = false
	if err := run.Validate(); !errors.Is(err, ErrInvalidToolCall) {
		t.Fatalf("read-only business object error = %v, want ErrInvalidToolCall", err)
	}
}

func TestCapturedRunRejectsCanonicalTokensWhenUsageIsIncomplete(t *testing.T) {
	run := validCapturedRun(t)
	run.ModelCallCount = intPtr(2)
	run.DroppedModelCalls = intPtr(1)
	run.MissingModelCalls = intPtr(1)
	if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("incomplete usage with canonical tokens error = %v, want ErrInvalidRun", err)
	}
	run.PromptTokens = nil
	run.CompletionTokens = nil
	run.TotalTokens = nil
	run.CachedTokens = nil
	run.ReasoningTokens = nil
	if err := run.Validate(); err != nil {
		t.Fatalf("incomplete usage with unknown canonical tokens rejected: %v", err)
	}
}

func TestValidateForCreateRejectsPreassignedPersistenceIdentities(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Run)
		wantErr error
	}{
		{
			name: "run id",
			mutate: func(run *Run) {
				run.ID = 41
			},
			wantErr: ErrInvalidRun,
		},
		{
			name: "tool id",
			mutate: func(run *Run) {
				run.ToolCalls[0].ID = 51
			},
			wantErr: ErrInvalidToolCall,
		},
		{
			name: "tool run id",
			mutate: func(run *Run) {
				run.ToolCalls[0].RunID = 41
			},
			wantErr: ErrInvalidToolCall,
		},
		{
			name: "matching persisted aggregate ids",
			mutate: func(run *Run) {
				run.ID = 41
				run.ToolCalls[0].ID = 51
				run.ToolCalls[0].RunID = 41
			},
			wantErr: ErrInvalidRun,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			run := validCapturedRun(t)
			test.mutate(run)
			if err := run.ValidateForCreate(); !errors.Is(err, test.wantErr) {
				t.Fatalf("ValidateForCreate() error = %v, want %v", err, test.wantErr)
			}
		})
	}

	if err := validCapturedRun(t).ValidateForCreate(); err != nil {
		t.Fatalf("zero persistence identities rejected: %v", err)
	}
}

func TestRunFailureFieldsMatchOutcome(t *testing.T) {
	for _, source := range []string{"main", "agent", "transport", "model", "tool"} {
		t.Run("accepted source "+source, func(t *testing.T) {
			input := validRunMetadata()
			input.Status = RunStatusError
			input.FailureSource = stringPtr(source)
			input.FailureCategory = stringPtr("execution")
			input.FailureCode = stringPtr("failed")
			input.FailureRetryable = boolPtr(false)
			if _, err := NewRun(input); err != nil {
				t.Fatalf("NewRun() error = %v", err)
			}
		})
	}

	t.Run("successful run rejects failure metadata", func(t *testing.T) {
		input := validRunMetadata()
		input.FailureSource = stringPtr("main")
		input.FailureCategory = stringPtr("execution")
		input.FailureCode = stringPtr("failed")
		input.FailureRetryable = boolPtr(false)
		if _, err := NewRun(input); !errors.Is(err, ErrInvalidRun) {
			t.Fatalf("NewRun() error = %v, want ErrInvalidRun", err)
		}
	})

	t.Run("failed run rejects incomplete metadata", func(t *testing.T) {
		input := validRunMetadata()
		input.Status = RunStatusError
		input.FailureSource = stringPtr("main")
		input.FailureCategory = stringPtr("execution")
		input.FailureCode = stringPtr("failed")
		if _, err := NewRun(input); !errors.Is(err, ErrInvalidRun) {
			t.Fatalf("NewRun() error = %v, want ErrInvalidRun", err)
		}
	})
}

func TestToolCallIDUsesBoundedCanonicalAlias(t *testing.T) {
	run := validCapturedRun(t)
	for _, callID := range []string{"call-1", "call-12345678901234567890"} {
		run.ToolCalls[0].CallID = stringPtr(callID)
		if err := run.Validate(); err != nil {
			t.Fatalf("CallID %q rejected: %v", callID, err)
		}
	}

	for _, callID := range []string{"", "call-0", "call-01", "call-a", "response-1", "call-123456789012345678901"} {
		run.ToolCalls[0].CallID = stringPtr(callID)
		if err := run.Validate(); !errors.Is(err, ErrInvalidToolCall) {
			t.Fatalf("CallID %q error = %v, want ErrInvalidToolCall", callID, err)
		}
	}

	run.ToolCalls[0].CallID = nil
	if err := run.Validate(); err != nil {
		t.Fatalf("nil CallID rejected: %v", err)
	}
}

func TestAuditEntitiesHaveNoFreeFormErrorPersistenceFields(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(Run{}), reflect.TypeOf(ToolCall{})} {
		for _, forbidden := range []string{"FailureMessage", "ErrorMessage", "ErrorParam", "Param", "Message"} {
			if _, exists := typ.FieldByName(forbidden); exists {
				t.Fatalf("%s unexpectedly exposes %s", typ.Name(), forbidden)
			}
		}
	}
	run := validCapturedRun(t)
	data, err := json.Marshal(run)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"provider secret", "raw parameter", "authorization"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("serialized audit leaked %q: %s", secret, data)
		}
	}
}

func validCapturedRun(t *testing.T) *Run {
	t.Helper()
	input := validRunMetadata()
	input.TelemetryStatus = TelemetryCaptured
	run, err := NewRun(input)
	if err != nil {
		t.Fatal(err)
	}
	agentStart := input.StartedAt.Add(10 * time.Millisecond)
	agentFinish := agentStart.Add(25 * time.Millisecond)
	agentDuration := agentFinish.Sub(agentStart).Milliseconds()
	usage := `{"reported_model_calls":1,"missing_model_calls":0,"reported_totals":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"cached_tokens":1,"reasoning_tokens":2},"complete_totals":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15,"cached_tokens":1,"reasoning_tokens":2}}`
	models := `[{"sequence":1,"provider":"openai_compatible","model":"test-model","status":"success"}]`
	result := `{"id":7,"status":"draft"}`
	run.AgentStartedAt = &agentStart
	run.AgentFinishedAt = &agentFinish
	run.AgentDurationMS = &agentDuration
	run.ModelCallCount = intPtr(1)
	run.ToolCallCount = intPtr(1)
	run.DroppedModelCalls = intPtr(0)
	run.DroppedToolCalls = intPtr(0)
	run.ReportedModelCalls = intPtr(1)
	run.MissingModelCalls = intPtr(0)
	run.PromptTokens = int64Ptr(10)
	run.CompletionTokens = int64Ptr(5)
	run.TotalTokens = int64Ptr(15)
	run.CachedTokens = int64Ptr(1)
	run.ReasoningTokens = int64Ptr(2)
	run.UsageJSON = &usage
	run.ModelCallsJSON = &models
	run.ToolCalls = []ToolCall{{
		Sequence:               2,
		CallID:                 stringPtr("call-1"),
		Name:                   "draft_announcement",
		IsWrite:                true,
		Status:                 "success",
		StartedAt:              agentStart.Add(5 * time.Millisecond),
		DurationMS:             3,
		ArgumentsJSON:          `{}`,
		ArgumentsOriginalBytes: 2,
		ArgumentsCapturedBytes: 2,
		ArgumentsRedacted:      true,
		ResultJSON:             result,
		ResultOriginalBytes:    int64(len(result)),
		ResultCapturedBytes:    int64(len(result)),
		ResultRedacted:         true,
		BusinessObjectType:     stringPtr("announcement_draft"),
		BusinessObjectID:       int64Ptr(7),
		BusinessObjectStatus:   stringPtr("draft"),
	}}
	return run
}

func validRunMetadata() RunMetadata {
	started := time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)
	return RunMetadata{
		RequestID:             strings.Repeat("a", 32),
		UserID:                int64Ptr(7),
		ConversationID:        int64Ptr(11),
		SessionID:             stringPtr("session-1"),
		ActorRole:             stringPtr("member"),
		Status:                RunStatusSuccess,
		TelemetryStatus:       TelemetryNotStarted,
		HTTPStatus:            intPtr(200),
		StartedAt:             started,
		FinishedAt:            started.Add(50 * time.Millisecond),
		ConversationPersisted: boolPtr(true),
		CreatedAt:             started.Add(time.Second),
	}
}

func intPtr(value int) *int          { return &value }
func int64Ptr(value int64) *int64    { return &value }
func boolPtr(value bool) *bool       { return &value }
func stringPtr(value string) *string { return &value }
