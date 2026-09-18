package agenthttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"SWPUCAT/internal/agent"
	"SWPUCAT/internal/agent/telemetry"

	"github.com/gin-gonic/gin"
)

const testRequestID = "0123456789abcdef0123456789abcdef"

type stubChatEngine struct {
	enabled  bool
	response agent.ChatResponse
	err      error
	calls    int
	requests []agent.ChatRequest
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "provider timeout with private detail" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func (e *stubChatEngine) Enabled() bool {
	return e.enabled
}

func (e *stubChatEngine) Chat(_ context.Context, req agent.ChatRequest) (agent.ChatResponse, error) {
	e.calls++
	e.requests = append(e.requests, req)
	return e.response, e.err
}

type decodedEnvelope struct {
	Code        int                     `json:"code"`
	Message     string                  `json:"message"`
	FailureCode failureCode             `json:"failure_code"`
	Data        json.RawMessage         `json:"data"`
	Audit       *telemetry.AuditSummary `json:"audit"`
}

func testSnapshot() telemetry.Snapshot {
	startedAt := time.Date(2026, time.September, 18, 1, 2, 3, 0, time.UTC)
	return telemetry.Snapshot{
		StartedAt:      startedAt,
		FinishedAt:     startedAt.Add(125 * time.Millisecond),
		DurationMS:     125,
		ModelCallCount: 2,
		ToolCallCount:  1,
		Usage: telemetry.UsageSummary{
			ReportedModelCalls: 1,
			MissingModelCalls:  1,
			ReportedTotals: telemetry.TokenUsage{
				PromptTokens:     11,
				CompletionTokens: 7,
				TotalTokens:      18,
			},
		},
		ModelCalls: []telemetry.ModelCall{{
			Sequence:   1,
			Provider:   "openai_compatible",
			Model:      "test-model",
			Status:     "success",
			StartedAt:  startedAt,
			DurationMS: 100,
		}},
		ToolCalls: []telemetry.ToolCall{{
			Sequence:   2,
			CallID:     "call-1",
			Name:       "get_personal_work_hours",
			Status:     "success",
			StartedAt:  startedAt.Add(100 * time.Millisecond),
			DurationMS: 25,
		}},
	}
}

func chatRouter(engine chatEngine, secret string) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.POST("/chat", newHandler(engine, secret).Chat)
	return router
}

func performChat(t *testing.T, router http.Handler, body, requestID, secret string) (*httptest.ResponseRecorder, decodedEnvelope) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/chat", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if requestID != "" {
		request.Header.Set("X-Request-ID", requestID)
	}
	if secret != "" {
		request.Header.Set("X-Agent-Internal-Secret", secret)
	}
	router.ServeHTTP(recorder, request)

	var envelope decodedEnvelope
	if err := json.Unmarshal(recorder.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return recorder, envelope
}

func assertAuditSnapshot(t *testing.T, audit *telemetry.AuditSummary, requestID string) {
	t.Helper()
	if audit == nil {
		t.Fatal("audit = nil, want audit metadata")
	}
	if audit.RequestID != requestID {
		t.Fatalf("audit.request_id = %q, want %q", audit.RequestID, requestID)
	}
	if audit.Telemetry == nil {
		t.Fatal("audit.telemetry = nil, want execution snapshot")
	}
	if audit.Telemetry.ModelCallCount != 2 || audit.Telemetry.ToolCallCount != 1 {
		t.Fatalf("audit.telemetry counts = (%d, %d), want (2, 1)", audit.Telemetry.ModelCallCount, audit.Telemetry.ToolCallCount)
	}
	if len(audit.Telemetry.ModelCalls) != 1 || audit.Telemetry.ModelCalls[0].Model != "test-model" {
		t.Fatalf("audit.telemetry.model_calls = %#v", audit.Telemetry.ModelCalls)
	}
	if len(audit.Telemetry.ToolCalls) != 1 || audit.Telemetry.ToolCalls[0].Name != "get_personal_work_hours" {
		t.Fatalf("audit.telemetry.tool_calls = %#v", audit.Telemetry.ToolCalls)
	}
}

func TestChatSuccessIncludesPrivateAuditOutsideData(t *testing.T) {
	engine := &stubChatEngine{
		enabled: true,
		response: agent.ChatResponse{
			Answer:    "已完成查询",
			SessionID: "session-1",
			Telemetry: testSnapshot(),
		},
	}
	recorder, envelope := performChat(t, chatRouter(engine, "internal-secret"), `{"message":"查询本周工时"}`, testRequestID, "internal-secret")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}
	if recorder.Header().Get("X-Request-ID") != testRequestID {
		t.Fatalf("X-Request-ID = %q, want %q", recorder.Header().Get("X-Request-ID"), testRequestID)
	}
	if envelope.Code != 0 || envelope.Message != "success" {
		t.Fatalf("envelope = %#v", envelope)
	}
	if envelope.FailureCode != "" || strings.Contains(recorder.Body.String(), "failure_code") {
		t.Fatalf("success response included failure code: %s", recorder.Body.String())
	}
	assertAuditSnapshot(t, envelope.Audit, testRequestID)
	if engine.calls != 1 || len(engine.requests) != 1 {
		t.Fatalf("engine calls = %d, requests = %d, want one", engine.calls, len(engine.requests))
	}
	if engine.requests[0].RequestID != testRequestID {
		t.Fatalf("engine request ID = %q, want %q", engine.requests[0].RequestID, testRequestID)
	}

	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		t.Fatalf("decode data: %v", err)
	}
	if data["answer"] != "已完成查询" || data["session_id"] != "session-1" {
		t.Fatalf("data = %#v", data)
	}
	for _, forbidden := range []string{"telemetry", "audit", "model_calls", "tool_calls"} {
		if _, ok := data[forbidden]; ok {
			t.Fatalf("data unexpectedly contains %q: %#v", forbidden, data)
		}
	}
}

func TestChatExecutionErrorsIncludeAudit(t *testing.T) {
	tests := []struct {
		name        string
		err         error
		wantStatus  int
		wantCode    int
		wantMessage string
		wantFailure failureCode
	}{
		{name: "busy", err: agent.ErrBusy, wantStatus: http.StatusServiceUnavailable, wantCode: 503, wantMessage: "agent is busy; please retry shortly", wantFailure: failureAgentBusy},
		{name: "queue timeout", err: agent.ErrQueueTimeout, wantStatus: http.StatusServiceUnavailable, wantCode: 503, wantMessage: "agent queue wait timed out; please retry shortly", wantFailure: failureQueueTimeout},
		{name: "disabled", err: agent.ErrDisabled, wantStatus: http.StatusServiceUnavailable, wantCode: 503, wantMessage: "agent is disabled", wantFailure: failureAgentDisabled},
		{name: "invalid message", err: agent.ErrInvalidMessage, wantStatus: http.StatusBadRequest, wantCode: 400, wantMessage: agent.ErrInvalidMessage.Error(), wantFailure: failureInvalidMessage},
		{name: "invalid session", err: agent.ErrInvalidSession, wantStatus: http.StatusBadRequest, wantCode: 400, wantMessage: agent.ErrInvalidSession.Error(), wantFailure: failureInvalidSession},
		{name: "invalid history", err: agent.ErrInvalidHistory, wantStatus: http.StatusBadRequest, wantCode: 400, wantMessage: agent.ErrInvalidHistory.Error(), wantFailure: failureInvalidHistory},
		{name: "ordinary failure", err: errors.New("provider response contained private detail"), wantStatus: http.StatusBadGateway, wantCode: 502, wantMessage: "agent execution failed", wantFailure: failureExecutionFailed},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &stubChatEngine{
				enabled:  true,
				response: agent.ChatResponse{Telemetry: testSnapshot()},
				err:      tt.err,
			}
			recorder, envelope := performChat(t, chatRouter(engine, "internal-secret"), `{"message":"执行任务"}`, testRequestID, "internal-secret")

			if recorder.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, tt.wantStatus, recorder.Body.String())
			}
			if envelope.Code != tt.wantCode || envelope.Message != tt.wantMessage {
				t.Fatalf("response = %#v, want code=%d message=%q", envelope, tt.wantCode, tt.wantMessage)
			}
			if envelope.FailureCode != tt.wantFailure {
				t.Fatalf("failure_code = %q, want %q", envelope.FailureCode, tt.wantFailure)
			}
			if len(envelope.Data) != 0 {
				t.Fatalf("data = %s, want omitted", envelope.Data)
			}
			assertAuditSnapshot(t, envelope.Audit, testRequestID)
			if strings.Contains(recorder.Body.String(), "provider response contained private detail") {
				t.Fatalf("response leaked internal error: %s", recorder.Body.String())
			}
		})
	}
}

func TestChatTimeoutsReturnGatewayTimeoutWithAudit(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "context deadline", err: context.DeadlineExceeded},
		{name: "network timeout", err: timeoutError{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := &stubChatEngine{
				enabled:  true,
				response: agent.ChatResponse{Telemetry: testSnapshot()},
				err:      test.err,
			}
			recorder, envelope := performChat(t, chatRouter(engine, "internal-secret"), `{"message":"执行较慢任务"}`, testRequestID, "internal-secret")

			if recorder.Code != http.StatusGatewayTimeout {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusGatewayTimeout, recorder.Body.String())
			}
			if envelope.Code != 504 || envelope.Message != "agent execution timed out" {
				t.Fatalf("response = %#v", envelope)
			}
			if envelope.FailureCode != failureExecutionTimeout {
				t.Fatalf("failure_code = %q, want %q", envelope.FailureCode, failureExecutionTimeout)
			}
			assertAuditSnapshot(t, envelope.Audit, testRequestID)
			if strings.Contains(recorder.Body.String(), "private detail") {
				t.Fatalf("timeout response leaked internal detail: %s", recorder.Body.String())
			}
		})
	}
}

func TestChatCancellationKeepsExistingGatewayResponseAndStableCode(t *testing.T) {
	engine := &stubChatEngine{
		enabled:  true,
		response: agent.ChatResponse{Telemetry: testSnapshot()},
		err:      context.Canceled,
	}
	recorder, envelope := performChat(t, chatRouter(engine, "internal-secret"), `{"message":"执行任务"}`, testRequestID, "internal-secret")

	if recorder.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadGateway, recorder.Body.String())
	}
	if envelope.Code != 502 || envelope.Message != "agent execution failed" || envelope.FailureCode != failureRequestCanceled {
		t.Fatalf("response = %#v", envelope)
	}
	assertAuditSnapshot(t, envelope.Audit, testRequestID)
}

func TestChatRejectsInvalidSecretWithoutExecutionOrAudit(t *testing.T) {
	engine := &stubChatEngine{
		enabled:  true,
		response: agent.ChatResponse{Telemetry: testSnapshot()},
	}
	recorder, envelope := performChat(t, chatRouter(engine, "expected-secret"), `{"message":"不应执行"}`, testRequestID, "wrong-secret")

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	if envelope.Code != 401 || envelope.Message != "invalid internal secret" {
		t.Fatalf("response = %#v", envelope)
	}
	if envelope.FailureCode != failureInvalidInternalSecret {
		t.Fatalf("failure_code = %q, want %q", envelope.FailureCode, failureInvalidInternalSecret)
	}
	if engine.calls != 0 {
		t.Fatalf("engine calls = %d, want 0", engine.calls)
	}
	if envelope.Audit != nil || strings.Contains(recorder.Body.String(), `"audit"`) || strings.Contains(recorder.Body.String(), `"telemetry"`) {
		t.Fatalf("unauthorized response leaked audit metadata: %s", recorder.Body.String())
	}
}

func TestNewHandlerNormalizesNilEngine(t *testing.T) {
	handler := NewHandler(nil, "internal-secret")
	router := gin.New()
	router.GET("/health", handler.Health)
	router.POST("/chat", handler.Chat)

	health := httptest.NewRecorder()
	router.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/health", nil))
	if health.Code != http.StatusOK || !strings.Contains(health.Body.String(), `"enabled":false`) {
		t.Fatalf("health response = %d %s", health.Code, health.Body.String())
	}

	recorder, envelope := performChat(t, router, `{"message":"hello"}`, testRequestID, "internal-secret")
	if recorder.Code != http.StatusServiceUnavailable || envelope.Code != 503 || envelope.Message != "agent is disabled" {
		t.Fatalf("chat response = %d %#v", recorder.Code, envelope)
	}
	if envelope.FailureCode != failureAgentDisabled {
		t.Fatalf("failure_code = %q, want %q", envelope.FailureCode, failureAgentDisabled)
	}
	if envelope.Audit == nil || envelope.Audit.RequestID != testRequestID || envelope.Audit.Telemetry != nil {
		t.Fatalf("nil-engine audit = %#v", envelope.Audit)
	}
}

func TestChatInvalidJSONReturnsOnlySafeCorrelationMetadata(t *testing.T) {
	engine := &stubChatEngine{enabled: true}
	recorder, envelope := performChat(t, chatRouter(engine, "internal-secret"), `{"message":`, testRequestID, "internal-secret")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
	}
	if envelope.Code != 400 || envelope.Message != "invalid request body" {
		t.Fatalf("response = %#v", envelope)
	}
	if envelope.FailureCode != failureInvalidRequestBody {
		t.Fatalf("failure_code = %q, want %q", envelope.FailureCode, failureInvalidRequestBody)
	}
	if engine.calls != 0 {
		t.Fatalf("engine calls = %d, want 0", engine.calls)
	}
	if len(envelope.Data) != 0 {
		t.Fatalf("data = %s, want omitted", envelope.Data)
	}
	if envelope.Audit == nil || envelope.Audit.RequestID != testRequestID {
		t.Fatalf("audit = %#v, want request ID only", envelope.Audit)
	}
	if envelope.Audit.Telemetry != nil {
		t.Fatalf("audit telemetry = %#v, want nil", envelope.Audit.Telemetry)
	}
	if strings.Contains(recorder.Body.String(), `"telemetry"`) || strings.Contains(recorder.Body.String(), `"model_calls"`) || strings.Contains(recorder.Body.String(), `"tool_calls"`) {
		t.Fatalf("invalid-body response leaked execution metadata: %s", recorder.Body.String())
	}
}

func TestChatValidatesRequestIDBeforeEchoOrExecution(t *testing.T) {
	tests := []struct {
		name       string
		requestID  string
		wantID     string
		wantHeader string
	}{
		{name: "valid lowercase hexadecimal", requestID: testRequestID, wantID: testRequestID, wantHeader: testRequestID},
		{name: "uppercase rejected", requestID: "0123456789ABCDEF0123456789ABCDEF"},
		{name: "non hexadecimal rejected", requestID: "g123456789abcdef0123456789abcdef"},
		{name: "short rejected", requestID: "0123456789abcdef"},
		{name: "long rejected", requestID: testRequestID + "00"},
		{name: "whitespace rejected", requestID: " " + testRequestID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			engine := &stubChatEngine{
				enabled:  true,
				response: agent.ChatResponse{Answer: "ok", SessionID: "session", Telemetry: testSnapshot()},
			}
			recorder, envelope := performChat(t, chatRouter(engine, "internal-secret"), `{"message":"检查 ID"}`, tt.requestID, "internal-secret")

			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d: %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			if engine.calls != 1 || len(engine.requests) != 1 {
				t.Fatalf("engine calls = %d, requests = %d, want one", engine.calls, len(engine.requests))
			}
			if engine.requests[0].RequestID != tt.wantID {
				t.Fatalf("engine request ID = %q, want %q", engine.requests[0].RequestID, tt.wantID)
			}
			if got := recorder.Header().Get("X-Request-ID"); got != tt.wantHeader {
				t.Fatalf("response X-Request-ID = %q, want %q", got, tt.wantHeader)
			}
			if envelope.Audit == nil || envelope.Audit.RequestID != tt.wantID {
				t.Fatalf("audit = %#v, want request ID %q", envelope.Audit, tt.wantID)
			}
			if tt.wantID == "" && strings.Contains(recorder.Body.String(), tt.requestID) {
				t.Fatalf("response echoed rejected request ID %q: %s", tt.requestID, recorder.Body.String())
			}
		})
	}
}
