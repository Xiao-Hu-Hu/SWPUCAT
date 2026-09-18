package http

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"SWPUCAT/internal/agent/telemetry"
	agentauditapp "SWPUCAT/internal/application/agentaudit"
	conversationapp "SWPUCAT/internal/application/conversation"
	"SWPUCAT/internal/domain/agentaudit"
	"SWPUCAT/internal/domain/conversation"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/infrastructure/agentclient"

	"github.com/gin-gonic/gin"
)

const agentAuditTestConversationID int64 = 73

type agentAuditTestClient struct {
	chat  func(context.Context, agentclient.ChatRequest, string, string) (agentclient.ChatResponse, error)
	calls int
}

func (c *agentAuditTestClient) Chat(ctx context.Context, req agentclient.ChatRequest, authorization, requestID string) (agentclient.ChatResponse, error) {
	c.calls++
	if c.chat == nil {
		return agentclient.ChatResponse{}, errors.New("unexpected agent call")
	}
	return c.chat(ctx, req, authorization, requestID)
}

type agentAuditTestConversationRepo struct {
	item          *conversation.Conversation
	findCalls     int
	findErrAt     int
	findErr       error
	createErr     error
	listErr       error
	recordErr     error
	recordCalls   int
	lastRecordCtx context.Context
}

func (r *agentAuditTestConversationRepo) Create(_ context.Context, item *conversation.Conversation) error {
	if r.createErr != nil {
		return r.createErr
	}
	if r.item != nil {
		return shared.ErrConflict
	}
	copy := *item
	copy.ID = agentAuditTestConversationID
	r.item = &copy
	item.ID = copy.ID
	return nil
}

func (r *agentAuditTestConversationRepo) FindBySession(_ context.Context, userID int64, sessionID string) (*conversation.Conversation, error) {
	r.findCalls++
	if r.findErrAt == r.findCalls && r.findErr != nil {
		return nil, r.findErr
	}
	if r.item == nil || r.item.UserID != userID || r.item.SessionID != sessionID {
		return nil, shared.ErrNotFound
	}
	copy := *r.item
	return &copy, nil
}

func (r *agentAuditTestConversationRepo) ListRecent(_ context.Context, _ int64, _ int) ([]*conversation.Conversation, error) {
	if r.item == nil {
		return []*conversation.Conversation{}, nil
	}
	copy := *r.item
	return []*conversation.Conversation{&copy}, nil
}

func (r *agentAuditTestConversationRepo) UpdateTitle(_ context.Context, userID int64, sessionID, title string) error {
	if r.item == nil || r.item.UserID != userID || r.item.SessionID != sessionID {
		return shared.ErrNotFound
	}
	r.item.Title = title
	return nil
}

func (r *agentAuditTestConversationRepo) ListRecentMessages(_ context.Context, userID int64, sessionID string, _ int) ([]*conversation.Message, error) {
	if r.listErr != nil {
		return nil, r.listErr
	}
	if r.item == nil || r.item.UserID != userID || r.item.SessionID != sessionID {
		return nil, shared.ErrNotFound
	}
	return []*conversation.Message{}, nil
}

func (r *agentAuditTestConversationRepo) RecordTurn(ctx context.Context, userID int64, sessionID, title string, titleCustomized bool, _, _ *conversation.Message, summary, lastSourcesJSON string, activeAt time.Time) error {
	r.recordCalls++
	r.lastRecordCtx = ctx
	if r.recordErr != nil {
		return r.recordErr
	}
	if r.item == nil || r.item.UserID != userID || r.item.SessionID != sessionID {
		return shared.ErrNotFound
	}
	r.item.Title = title
	r.item.TitleCustomized = titleCustomized
	return r.item.UpdateContext(summary, lastSourcesJSON, activeAt)
}

type agentAuditContextKey struct{}

type agentAuditTestRepo struct {
	runs              []*agentaudit.Run
	err               error
	ctxErrAtCreate    error
	hadDeadline       bool
	deadlineRemaining time.Duration
	contextValue      any
}

func (r *agentAuditTestRepo) CreateRun(ctx context.Context, run *agentaudit.Run) error {
	r.ctxErrAtCreate = ctx.Err()
	deadline, hasDeadline := ctx.Deadline()
	r.hadDeadline = hasDeadline
	if hasDeadline {
		r.deadlineRemaining = time.Until(deadline)
	}
	r.contextValue = ctx.Value(agentAuditContextKey{})
	r.runs = append(r.runs, run)
	return r.err
}

func newAgentAuditTestEngine(client agentChatClient, conversations *agentAuditTestConversationRepo, audits *agentAuditTestRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	if conversations == nil {
		conversations = &agentAuditTestConversationRepo{}
	}
	var auditService *agentauditapp.Service
	if audits != nil {
		auditService = agentauditapp.NewService(audits)
	}
	handler := newAgentHandler(client, conversationapp.NewService(conversations), nil, auditService)
	engine := gin.New()
	engine.Use(requestIDMiddleware(bytes.NewReader(bytes.Repeat([]byte{0x42}, 16))))
	engine.POST("/api/agent/chat", func(c *gin.Context) {
		c.Set(ContextUserID, int64(7))
		c.Set(ContextRole, "member")
		handler.Chat(c)
	})
	return engine
}

func performAgentAuditTestChat(engine *gin.Engine, ctx context.Context, body string) *httptest.ResponseRecorder {
	if ctx == nil {
		ctx = context.Background()
	}
	request := httptest.NewRequest(http.MethodPost, "/api/agent/chat", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer public-token")
	request.Header.Set("X-Request-ID", strings.Repeat("f", 32))
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func agentAuditTestSnapshot() *telemetry.Snapshot {
	collector := telemetry.NewCollector()
	snapshot := collector.Snapshot()
	return &snapshot
}

func onlyAgentAuditRun(t *testing.T, repo *agentAuditTestRepo) *agentaudit.Run {
	t.Helper()
	if repo == nil || len(repo.runs) != 1 {
		t.Fatalf("audit run count = %d, want 1", len(repo.runs))
	}
	return repo.runs[0]
}

func assertAgentAuditFailure(t *testing.T, run *agentaudit.Run, status agentaudit.RunStatus, source, category, code string, retryable bool) {
	t.Helper()
	if run.Status != status {
		t.Fatalf("run status = %q, want %q", run.Status, status)
	}
	if run.FailureSource == nil || *run.FailureSource != source || run.FailureCategory == nil || *run.FailureCategory != category || run.FailureCode == nil || *run.FailureCode != code || run.FailureRetryable == nil || *run.FailureRetryable != retryable {
		t.Fatalf("failure = source=%v category=%v code=%v retryable=%v", run.FailureSource, run.FailureCategory, run.FailureCode, run.FailureRetryable)
	}
}

func TestAgentChatAuditRecordsCapturedSuccessAndConversationDurability(t *testing.T) {
	client := &agentAuditTestClient{chat: func(_ context.Context, _ agentclient.ChatRequest, _, requestID string) (agentclient.ChatResponse, error) {
		return agentclient.ChatResponse{
			Answer:    "done",
			SessionID: "private-session",
			Audit:     &telemetry.AuditSummary{RequestID: requestID, Telemetry: agentAuditTestSnapshot()},
		}, nil
	}}
	conversationRepo := &agentAuditTestConversationRepo{}
	auditRepo := &agentAuditTestRepo{}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, conversationRepo, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	run := onlyAgentAuditRun(t, auditRepo)
	if run.RequestID != response.Header().Get("X-Request-ID") || run.UserID == nil || *run.UserID != 7 || run.ActorRole == nil || *run.ActorRole != "member" {
		t.Fatalf("run identity = request %q user %v role %v", run.RequestID, run.UserID, run.ActorRole)
	}
	if run.ConversationID == nil || *run.ConversationID != agentAuditTestConversationID || run.SessionID == nil || *run.SessionID != "session-7" {
		t.Fatalf("conversation identity = id %v session %v", run.ConversationID, run.SessionID)
	}
	if run.Status != agentaudit.RunStatusSuccess || run.HTTPStatus == nil || *run.HTTPStatus != http.StatusOK || run.FailureCode != nil {
		t.Fatalf("success outcome = status %q HTTP %v failure %v", run.Status, run.HTTPStatus, run.FailureCode)
	}
	if run.ConversationPersisted == nil || !*run.ConversationPersisted || conversationRepo.recordCalls != 1 {
		t.Fatalf("conversation persistence = audit %v calls %d", run.ConversationPersisted, conversationRepo.recordCalls)
	}
	if run.TelemetryStatus != agentaudit.TelemetryCaptured || run.ModelCallCount == nil || *run.ModelCallCount != 0 || run.ToolCallCount == nil || *run.ToolCallCount != 0 {
		t.Fatalf("telemetry = status %q models %v tools %v", run.TelemetryStatus, run.ModelCallCount, run.ToolCallCount)
	}
	if !auditRepo.hadDeadline || auditRepo.deadlineRemaining <= 0 || auditRepo.deadlineRemaining > agentAuditWriteTimeout || auditRepo.ctxErrAtCreate != nil {
		t.Fatalf("audit context = deadline %v remaining %s err %v", auditRepo.hadDeadline, auditRepo.deadlineRemaining, auditRepo.ctxErrAtCreate)
	}
}

func TestAgentChatAuditRecordsEveryLocalReturn(t *testing.T) {
	wantErr := errors.New("repository failed")
	tests := []struct {
		name               string
		body               string
		clientConfigured   bool
		configureRepo      func(*agentAuditTestConversationRepo)
		wantHTTP           int
		wantStatus         agentaudit.RunStatus
		wantSource         string
		wantCategory       string
		wantCode           string
		wantRetryable      bool
		wantSession        bool
		wantConversationID bool
	}{
		{name: "not configured", body: `{"message":"hello","session_id":"session-7"}`, wantHTTP: http.StatusServiceUnavailable, wantStatus: agentaudit.RunStatusUnavailable, wantSource: "main", wantCategory: "configuration", wantCode: "agent_not_configured"},
		{name: "invalid body", body: `{`, clientConfigured: true, wantHTTP: http.StatusBadRequest, wantStatus: agentaudit.RunStatusRejected, wantSource: "main", wantCategory: "validation", wantCode: "invalid_request_body"},
		{name: "invalid message", body: `{"message":"   "}`, clientConfigured: true, wantHTTP: http.StatusBadRequest, wantStatus: agentaudit.RunStatusRejected, wantSource: "main", wantCategory: "validation", wantCode: "invalid_message"},
		{name: "invalid session", body: `{"message":"hello","session_id":"bad/session"}`, clientConfigured: true, wantHTTP: http.StatusBadRequest, wantStatus: agentaudit.RunStatusRejected, wantSource: "main", wantCategory: "validation", wantCode: "invalid_session"},
		{
			name:             "conversation init",
			body:             `{"message":"hello","session_id":"session-7"}`,
			clientConfigured: true,
			configureRepo: func(repo *agentAuditTestConversationRepo) {
				repo.findErrAt, repo.findErr = 1, wantErr
			},
			wantHTTP:      http.StatusInternalServerError,
			wantStatus:    agentaudit.RunStatusError,
			wantSource:    "main",
			wantCategory:  "persistence",
			wantCode:      "conversation_initialize_failed",
			wantRetryable: true,
			wantSession:   true,
		},
		{
			name:             "conversation init canceled",
			body:             `{"message":"hello","session_id":"session-7"}`,
			clientConfigured: true,
			configureRepo: func(repo *agentAuditTestConversationRepo) {
				repo.findErrAt, repo.findErr = 1, context.Canceled
			},
			wantHTTP:     clientClosedRequestStatus,
			wantStatus:   agentaudit.RunStatusCanceled,
			wantSource:   "main",
			wantCategory: "persistence",
			wantCode:     "conversation_initialize_canceled",
			wantSession:  true,
		},
		{
			name:             "conversation init timeout",
			body:             `{"message":"hello","session_id":"session-7"}`,
			clientConfigured: true,
			configureRepo: func(repo *agentAuditTestConversationRepo) {
				repo.findErrAt, repo.findErr = 1, context.DeadlineExceeded
			},
			wantHTTP:      http.StatusGatewayTimeout,
			wantStatus:    agentaudit.RunStatusTimeout,
			wantSource:    "main",
			wantCategory:  "persistence",
			wantCode:      "conversation_initialize_timeout",
			wantRetryable: true,
			wantSession:   true,
		},
		{
			name:             "conversation context load",
			body:             `{"message":"hello","session_id":"session-7"}`,
			clientConfigured: true,
			configureRepo: func(repo *agentAuditTestConversationRepo) {
				repo.listErr = wantErr
			},
			wantHTTP:           http.StatusInternalServerError,
			wantStatus:         agentaudit.RunStatusError,
			wantSource:         "main",
			wantCategory:       "persistence",
			wantCode:           "conversation_load_failed",
			wantRetryable:      true,
			wantSession:        true,
			wantConversationID: true,
		},
		{
			name:             "conversation context load canceled",
			body:             `{"message":"hello","session_id":"session-7"}`,
			clientConfigured: true,
			configureRepo: func(repo *agentAuditTestConversationRepo) {
				repo.listErr = context.Canceled
			},
			wantHTTP:           clientClosedRequestStatus,
			wantStatus:         agentaudit.RunStatusCanceled,
			wantSource:         "main",
			wantCategory:       "persistence",
			wantCode:           "conversation_load_canceled",
			wantSession:        true,
			wantConversationID: true,
		},
		{
			name:             "conversation context load timeout",
			body:             `{"message":"hello","session_id":"session-7"}`,
			clientConfigured: true,
			configureRepo: func(repo *agentAuditTestConversationRepo) {
				repo.listErr = context.DeadlineExceeded
			},
			wantHTTP:           http.StatusGatewayTimeout,
			wantStatus:         agentaudit.RunStatusTimeout,
			wantSource:         "main",
			wantCategory:       "persistence",
			wantCode:           "conversation_load_timeout",
			wantRetryable:      true,
			wantSession:        true,
			wantConversationID: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			conversationRepo := &agentAuditTestConversationRepo{}
			if test.configureRepo != nil {
				test.configureRepo(conversationRepo)
			}
			client := &agentAuditTestClient{chat: func(context.Context, agentclient.ChatRequest, string, string) (agentclient.ChatResponse, error) {
				return agentclient.ChatResponse{}, errors.New("unexpected agent call")
			}}
			var boundary agentChatClient
			if test.clientConfigured {
				boundary = client
			}
			auditRepo := &agentAuditTestRepo{}
			response := performAgentAuditTestChat(newAgentAuditTestEngine(boundary, conversationRepo, auditRepo), nil, test.body)

			if response.Code != test.wantHTTP {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantHTTP, response.Body.String())
			}
			if client.calls != 0 {
				t.Fatalf("agent call count = %d, want 0", client.calls)
			}
			run := onlyAgentAuditRun(t, auditRepo)
			if run.HTTPStatus == nil || *run.HTTPStatus != test.wantHTTP || run.TelemetryStatus != agentaudit.TelemetryNotStarted || run.ConversationPersisted != nil {
				t.Fatalf("run metadata = HTTP %v telemetry %q persisted %v", run.HTTPStatus, run.TelemetryStatus, run.ConversationPersisted)
			}
			assertAgentAuditFailure(t, run, test.wantStatus, test.wantSource, test.wantCategory, test.wantCode, test.wantRetryable)
			if (run.SessionID != nil) != test.wantSession || (run.ConversationID != nil) != test.wantConversationID {
				t.Fatalf("run conversation = session %v conversation %v", run.SessionID, run.ConversationID)
			}
		})
	}
}

func TestAgentChatAuditMapsRawCanceledClientCallTo499(t *testing.T) {
	client := &agentAuditTestClient{chat: func(context.Context, agentclient.ChatRequest, string, string) (agentclient.ChatResponse, error) {
		return agentclient.ChatResponse{}, context.Canceled
	}}
	auditRepo := &agentAuditTestRepo{}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, &agentAuditTestConversationRepo{}, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != clientClosedRequestStatus || !strings.Contains(response.Body.String(), `"code":499`) {
		t.Fatalf("response = status %d body %s", response.Code, response.Body.String())
	}
	run := onlyAgentAuditRun(t, auditRepo)
	assertAgentAuditFailure(t, run, agentaudit.RunStatusCanceled, "transport", "canceled", "request_canceled", false)
	if run.TelemetryStatus != agentaudit.TelemetryUnavailable || run.ConversationPersisted != nil {
		t.Fatalf("run = telemetry %q persisted %v", run.TelemetryStatus, run.ConversationPersisted)
	}
}

func TestAgentChatAuditClassifiesStableClientFailures(t *testing.T) {
	type auditMode uint8
	const (
		auditUnavailable auditMode = iota
		auditNotStarted
		auditCaptured
	)
	tests := []struct {
		name           string
		kind           agentclient.ErrorKind
		code           agentclient.FailureCode
		downstreamHTTP int
		wantHTTP       int
		wantStatus     agentaudit.RunStatus
		wantSource     string
		wantCategory   string
		wantCode       string
		wantRetryable  bool
		auditMode      auditMode
	}{
		{name: "validation 4xx", kind: agentclient.KindDownstream, code: agentclient.FailureCodeInvalidMessage, downstreamHTTP: http.StatusBadRequest, wantHTTP: http.StatusBadRequest, wantStatus: agentaudit.RunStatusRejected, wantSource: "agent", wantCategory: "validation", wantCode: "invalid_message", auditMode: auditCaptured},
		{name: "busy", kind: agentclient.KindDownstream, code: agentclient.FailureCodeAgentBusy, downstreamHTTP: http.StatusServiceUnavailable, wantHTTP: http.StatusServiceUnavailable, wantStatus: agentaudit.RunStatusBusy, wantSource: "agent", wantCategory: "capacity", wantCode: "agent_busy", wantRetryable: true, auditMode: auditCaptured},
		{name: "queue timeout", kind: agentclient.KindDownstream, code: agentclient.FailureCodeQueueTimeout, downstreamHTTP: http.StatusServiceUnavailable, wantHTTP: http.StatusServiceUnavailable, wantStatus: agentaudit.RunStatusBusy, wantSource: "agent", wantCategory: "capacity", wantCode: "queue_timeout", wantRetryable: true, auditMode: auditCaptured},
		{name: "disabled", kind: agentclient.KindDownstream, code: agentclient.FailureCodeAgentDisabled, downstreamHTTP: http.StatusServiceUnavailable, wantHTTP: http.StatusServiceUnavailable, wantStatus: agentaudit.RunStatusUnavailable, wantSource: "agent", wantCategory: "availability", wantCode: "agent_disabled", auditMode: auditNotStarted},
		{name: "agent timeout", kind: agentclient.KindDownstream, code: agentclient.FailureCodeExecutionTimeout, downstreamHTTP: http.StatusGatewayTimeout, wantHTTP: http.StatusGatewayTimeout, wantStatus: agentaudit.RunStatusTimeout, wantSource: "agent", wantCategory: "timeout", wantCode: "execution_timeout", wantRetryable: true, auditMode: auditCaptured},
		{name: "agent canceled", kind: agentclient.KindDownstream, code: agentclient.FailureCodeRequestCanceled, downstreamHTTP: http.StatusBadGateway, wantHTTP: http.StatusBadGateway, wantStatus: agentaudit.RunStatusCanceled, wantSource: "agent", wantCategory: "canceled", wantCode: "request_canceled", auditMode: auditCaptured},
		{name: "agent 5xx", kind: agentclient.KindDownstream, code: agentclient.FailureCodeExecutionFailed, downstreamHTTP: http.StatusBadGateway, wantHTTP: http.StatusBadGateway, wantStatus: agentaudit.RunStatusError, wantSource: "agent", wantCategory: "execution", wantCode: "execution_failed", auditMode: auditCaptured},
		{name: "transport timeout", kind: agentclient.KindTimeout, code: agentclient.FailureCodeExecutionTimeout, wantHTTP: http.StatusGatewayTimeout, wantStatus: agentaudit.RunStatusTimeout, wantSource: "transport", wantCategory: "timeout", wantCode: "execution_timeout", wantRetryable: true},
		{name: "transport canceled", kind: agentclient.KindCanceled, code: agentclient.FailureCodeRequestCanceled, wantHTTP: clientClosedRequestStatus, wantStatus: agentaudit.RunStatusCanceled, wantSource: "transport", wantCategory: "canceled", wantCode: "request_canceled"},
		{name: "network transport", kind: agentclient.KindTransport, code: agentclient.FailureCodeTransport, wantHTTP: http.StatusInternalServerError, wantStatus: agentaudit.RunStatusUnavailable, wantSource: "transport", wantCategory: "network", wantCode: "transport_error", wantRetryable: true},
		{name: "protocol", kind: agentclient.KindProtocol, code: agentclient.FailureCodeProtocol, downstreamHTTP: http.StatusBadGateway, wantHTTP: http.StatusBadGateway, wantStatus: agentaudit.RunStatusError, wantSource: "transport", wantCategory: "protocol", wantCode: "protocol_error", auditMode: auditCaptured},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &agentAuditTestClient{chat: func(_ context.Context, _ agentclient.ChatRequest, _, requestID string) (agentclient.ChatResponse, error) {
				var audit *telemetry.AuditSummary
				switch test.auditMode {
				case auditNotStarted:
					audit = &telemetry.AuditSummary{RequestID: requestID}
				case auditCaptured:
					audit = &telemetry.AuditSummary{RequestID: requestID, Telemetry: agentAuditTestSnapshot()}
				}
				return agentclient.ChatResponse{}, &agentclient.ClientError{
					Kind:        test.kind,
					FailureCode: test.code,
					StatusCode:  test.downstreamHTTP,
					Message:     "misleading message that must not determine classification",
					RequestID:   requestID,
					Audit:       audit,
				}
			}}
			conversationRepo := &agentAuditTestConversationRepo{}
			auditRepo := &agentAuditTestRepo{}
			response := performAgentAuditTestChat(newAgentAuditTestEngine(client, conversationRepo, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

			if response.Code != test.wantHTTP {
				t.Fatalf("status = %d, want %d: %s", response.Code, test.wantHTTP, response.Body.String())
			}
			run := onlyAgentAuditRun(t, auditRepo)
			assertAgentAuditFailure(t, run, test.wantStatus, test.wantSource, test.wantCategory, test.wantCode, test.wantRetryable)
			wantTelemetry := agentaudit.TelemetryUnavailable
			if test.auditMode == auditNotStarted {
				wantTelemetry = agentaudit.TelemetryNotStarted
			} else if test.auditMode == auditCaptured {
				wantTelemetry = agentaudit.TelemetryCaptured
			}
			if run.TelemetryStatus != wantTelemetry || run.ConversationPersisted != nil || run.ConversationID == nil || *run.ConversationID != agentAuditTestConversationID {
				t.Fatalf("telemetry/durability = telemetry %q persisted %v conversation %v", run.TelemetryStatus, run.ConversationPersisted, run.ConversationID)
			}
			if conversationRepo.recordCalls != 0 {
				t.Fatalf("RecordTurn calls = %d, want 0", conversationRepo.recordCalls)
			}
		})
	}
}

func TestAgentChatAuditDoesNotInferLegacyFailureFromMessage(t *testing.T) {
	client := &agentAuditTestClient{chat: func(_ context.Context, _ agentclient.ChatRequest, _, requestID string) (agentclient.ChatResponse, error) {
		return agentclient.ChatResponse{}, &agentclient.HTTPError{
			StatusCode: http.StatusServiceUnavailable,
			Message:    "agent is busy and this wording must be ignored",
			RequestID:  requestID,
		}
	}}
	auditRepo := &agentAuditTestRepo{}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, &agentAuditTestConversationRepo{}, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	run := onlyAgentAuditRun(t, auditRepo)
	assertAgentAuditFailure(t, run, agentaudit.RunStatusUnavailable, "agent", "availability", "legacy", true)
	if run.TelemetryStatus != agentaudit.TelemetryUnavailable {
		t.Fatalf("telemetry status = %q", run.TelemetryStatus)
	}
}

func TestAgentChatAuditMarksLegacySuccessTelemetryUnavailable(t *testing.T) {
	client := &agentAuditTestClient{chat: func(context.Context, agentclient.ChatRequest, string, string) (agentclient.ChatResponse, error) {
		return agentclient.ChatResponse{Answer: "done", SessionID: "private-session"}, nil
	}}
	auditRepo := &agentAuditTestRepo{}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, &agentAuditTestConversationRepo{}, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	run := onlyAgentAuditRun(t, auditRepo)
	if run.Status != agentaudit.RunStatusSuccess || run.TelemetryStatus != agentaudit.TelemetryUnavailable || run.ConversationPersisted == nil || !*run.ConversationPersisted {
		t.Fatalf("run = status %q telemetry %q persisted %v", run.Status, run.TelemetryStatus, run.ConversationPersisted)
	}
}

func TestAgentChatAuditMarksSuccessfulEmptyTelemetryInvalid(t *testing.T) {
	client := &agentAuditTestClient{chat: func(_ context.Context, _ agentclient.ChatRequest, _, requestID string) (agentclient.ChatResponse, error) {
		return agentclient.ChatResponse{
			Answer:    "done",
			SessionID: "private-session",
			Audit:     &telemetry.AuditSummary{RequestID: requestID},
		}, nil
	}}
	auditRepo := &agentAuditTestRepo{}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, &agentAuditTestConversationRepo{}, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	run := onlyAgentAuditRun(t, auditRepo)
	if run.Status != agentaudit.RunStatusSuccess || run.TelemetryStatus != agentaudit.TelemetryInvalid || run.ModelCallCount != nil || run.ConversationPersisted == nil || !*run.ConversationPersisted {
		t.Fatalf("run = status %q telemetry %q models %v persisted %v", run.Status, run.TelemetryStatus, run.ModelCallCount, run.ConversationPersisted)
	}
}

func TestAgentChatAuditConversationPersistenceFailureDoesNotChangeSuccess(t *testing.T) {
	client := &agentAuditTestClient{chat: func(_ context.Context, _ agentclient.ChatRequest, _, requestID string) (agentclient.ChatResponse, error) {
		return agentclient.ChatResponse{Answer: "done", SessionID: "private-session", Audit: &telemetry.AuditSummary{RequestID: requestID, Telemetry: agentAuditTestSnapshot()}}, nil
	}}
	conversationRepo := &agentAuditTestConversationRepo{recordErr: errors.New("write failed")}
	auditRepo := &agentAuditTestRepo{}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, conversationRepo, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != http.StatusOK || response.Header().Get("X-Agent-Conversation-Persisted") != "false" {
		t.Fatalf("response = status %d persisted header %q body %s", response.Code, response.Header().Get("X-Agent-Conversation-Persisted"), response.Body.String())
	}
	run := onlyAgentAuditRun(t, auditRepo)
	if run.Status != agentaudit.RunStatusSuccess || run.FailureCode != nil || run.ConversationPersisted == nil || *run.ConversationPersisted {
		t.Fatalf("run = status %q failure %v persisted %v", run.Status, run.FailureCode, run.ConversationPersisted)
	}
}

func TestAgentChatAuditWriteFailureDoesNotChangeBusinessResponse(t *testing.T) {
	client := &agentAuditTestClient{chat: func(_ context.Context, _ agentclient.ChatRequest, _, requestID string) (agentclient.ChatResponse, error) {
		return agentclient.ChatResponse{Answer: "done", SessionID: "private-session", Audit: &telemetry.AuditSummary{RequestID: requestID, Telemetry: agentAuditTestSnapshot()}}, nil
	}}
	auditRepo := &agentAuditTestRepo{err: errors.New("audit store unavailable")}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, &agentAuditTestConversationRepo{}, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"code":0`) {
		t.Fatalf("response changed by audit failure: status %d body %s", response.Code, response.Body.String())
	}
	onlyAgentAuditRun(t, auditRepo)
}

func TestAgentChatAuditUsesWithoutCancelAndAddsDeadline(t *testing.T) {
	requestCtx, cancelRequest := context.WithCancel(context.WithValue(context.Background(), agentAuditContextKey{}, "preserved"))
	defer cancelRequest()
	client := &agentAuditTestClient{chat: func(context.Context, agentclient.ChatRequest, string, string) (agentclient.ChatResponse, error) {
		cancelRequest()
		return agentclient.ChatResponse{Answer: "done", SessionID: "private-session"}, nil
	}}
	auditRepo := &agentAuditTestRepo{}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, &agentAuditTestConversationRepo{}, auditRepo), requestCtx, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if requestCtx.Err() != context.Canceled {
		t.Fatalf("request context error = %v", requestCtx.Err())
	}
	if auditRepo.ctxErrAtCreate != nil || !auditRepo.hadDeadline || auditRepo.deadlineRemaining <= 0 || auditRepo.deadlineRemaining > agentAuditWriteTimeout || auditRepo.contextValue != "preserved" {
		t.Fatalf("audit context = err %v deadline %v remaining %s value %#v", auditRepo.ctxErrAtCreate, auditRepo.hadDeadline, auditRepo.deadlineRemaining, auditRepo.contextValue)
	}
}

func TestAgentChatAuditRejectsMismatchedSnapshotCorrelation(t *testing.T) {
	client := &agentAuditTestClient{chat: func(context.Context, agentclient.ChatRequest, string, string) (agentclient.ChatResponse, error) {
		return agentclient.ChatResponse{
			Answer:    "done",
			SessionID: "private-session",
			Audit:     &telemetry.AuditSummary{RequestID: strings.Repeat("a", 32), Telemetry: agentAuditTestSnapshot()},
		}, nil
	}}
	auditRepo := &agentAuditTestRepo{}
	response := performAgentAuditTestChat(newAgentAuditTestEngine(client, &agentAuditTestConversationRepo{}, auditRepo), nil, `{"message":"hello","session_id":"session-7"}`)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	run := onlyAgentAuditRun(t, auditRepo)
	if run.Status != agentaudit.RunStatusSuccess || run.TelemetryStatus != agentaudit.TelemetryInvalid || run.ModelCallCount != nil || run.AgentStartedAt != nil {
		t.Fatalf("run = status %q telemetry %q models %v agent started %v", run.Status, run.TelemetryStatus, run.ModelCallCount, run.AgentStartedAt)
	}
}
