package http

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"SWPUCAT/internal/agent/telemetry"
	conversationapp "SWPUCAT/internal/application/conversation"
	"SWPUCAT/internal/domain/conversation"
	"SWPUCAT/internal/domain/shared"
	"SWPUCAT/internal/infrastructure/agentclient"

	"github.com/gin-gonic/gin"
)

var requestIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func TestRequestIDMiddlewareIgnoresInboundIDAndPropagatesTrustedID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	randomBytes := append(bytes.Repeat([]byte{0x11}, 16), bytes.Repeat([]byte{0x22}, 16)...)
	engine := gin.New()
	engine.Use(requestIDMiddleware(bytes.NewReader(randomBytes)))

	var captured []string
	engine.GET("/capture", func(c *gin.Context) {
		fromRequestContext, _ := c.Request.Context().Value(requestIDContextKey{}).(string)
		if fromRequestContext != GetRequestID(c) {
			t.Errorf("request context ID = %q, gin context ID = %q", fromRequestContext, GetRequestID(c))
		}
		if fromHeader := c.GetHeader("X-Request-ID"); fromHeader != GetRequestID(c) {
			t.Errorf("request header ID = %q, trusted context ID = %q", fromHeader, GetRequestID(c))
		}
		captured = append(captured, GetRequestID(c))
		c.Status(http.StatusNoContent)
	})

	forged := strings.Repeat("a", 32)
	wants := []string{
		hex.EncodeToString(bytes.Repeat([]byte{0x11}, 16)),
		hex.EncodeToString(bytes.Repeat([]byte{0x22}, 16)),
	}
	for index, want := range wants {
		request := httptest.NewRequest(http.MethodGet, "/capture", nil)
		request.Header.Set("X-Request-ID", forged)
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, request)

		if response.Code != http.StatusNoContent {
			t.Fatalf("request %d status = %d", index+1, response.Code)
		}
		if got := response.Header().Get("X-Request-ID"); got != want {
			t.Fatalf("request %d response ID = %q, want %q", index+1, got, want)
		}
		if !requestIDPattern.MatchString(want) || want == forged {
			t.Fatalf("request %d trusted ID = %q", index+1, want)
		}
	}

	if len(captured) != 2 || captured[0] != wants[0] || captured[1] != wants[1] {
		t.Fatalf("downstream IDs = %v, want %v", captured, wants)
	}
	if captured[0] == captured[1] {
		t.Fatalf("two requests with the same forged header reused ID %q", captured[0])
	}
}

func TestRequestIDHeaderSurvivesUnauthorizedAndNotFoundResponses(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(RequestIDMiddleware())
	engine.GET("/protected", JWTAuthMiddleware(nil, nil), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})
	engine.GET("/forbidden", RBACMiddleware("captain"), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	for _, test := range []struct {
		name       string
		path       string
		wantStatus int
	}{
		{name: "unauthorized", path: "/protected", wantStatus: http.StatusUnauthorized},
		{name: "forbidden", path: "/forbidden", wantStatus: http.StatusForbidden},
		{name: "not-found", path: "/missing", wantStatus: http.StatusNotFound},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.Code, test.wantStatus)
			}
			if requestID := response.Header().Get("X-Request-ID"); !requestIDPattern.MatchString(requestID) {
				t.Fatalf("X-Request-ID = %q", requestID)
			}
		})
	}
}

func TestRequestIDHeaderSurvivesGinRecovery(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	// This is the same relative ordering used by gin.Default in NewRouter:
	// Recovery wraps the trusted request-ID middleware and downstream handlers.
	engine.Use(gin.Recovery(), RequestIDMiddleware())
	engine.GET("/panic", func(_ *gin.Context) {
		panic("private panic detail")
	})

	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/panic", nil))
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if requestID := response.Header().Get("X-Request-ID"); !requestIDPattern.MatchString(requestID) {
		t.Fatalf("X-Request-ID = %q", requestID)
	}
}

func TestRouterExposesRequestIDToBrowserClients(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := NewRouter(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil).Setup()
	request := httptest.NewRequest(http.MethodGet, "/missing", nil)
	request.Header.Set("Origin", "https://example.test")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)

	if requestID := response.Header().Get("X-Request-ID"); !requestIDPattern.MatchString(requestID) {
		t.Fatalf("X-Request-ID = %q", requestID)
	}
	exposed := strings.ToLower(response.Header().Get("Access-Control-Expose-Headers"))
	if !strings.Contains(exposed, "x-request-id") {
		t.Fatalf("Access-Control-Expose-Headers = %q", exposed)
	}
}

type publicAgentClient struct {
	response   agentclient.ChatResponse
	err        error
	requestIDs []string
}

func (c *publicAgentClient) Chat(_ context.Context, _ agentclient.ChatRequest, _, requestID string) (agentclient.ChatResponse, error) {
	c.requestIDs = append(c.requestIDs, requestID)
	return c.response, c.err
}

type publicAgentConversationRepo struct {
	conversation.Repository
	item     *conversation.Conversation
	messages []*conversation.Message
	nextID   int64
	feedback *conversation.ResponseFeedback
}

func (r *publicAgentConversationRepo) Create(_ context.Context, item *conversation.Conversation) error {
	if r.item != nil {
		return shared.ErrConflict
	}
	if r.nextID == 0 {
		r.nextID = 1
	}
	item.ID = r.nextID
	r.nextID++
	copy := *item
	r.item = &copy
	return nil
}

func (r *publicAgentConversationRepo) FindBySession(_ context.Context, userID int64, sessionID string) (*conversation.Conversation, error) {
	if r.item == nil || r.item.UserID != userID || r.item.SessionID != sessionID {
		return nil, shared.ErrNotFound
	}
	copy := *r.item
	return &copy, nil
}

func (r *publicAgentConversationRepo) ListRecentMessages(_ context.Context, userID int64, sessionID string, _ int) ([]*conversation.Message, error) {
	if r.item == nil || r.item.UserID != userID || r.item.SessionID != sessionID {
		return nil, shared.ErrNotFound
	}
	result := make([]*conversation.Message, 0, len(r.messages))
	for _, message := range r.messages {
		copy := *message
		result = append(result, &copy)
	}
	return result, nil
}

func (r *publicAgentConversationRepo) RecordTurn(_ context.Context, userID int64, sessionID, title string, titleCustomized bool, userMessage, assistantMessage *conversation.Message, summary, sources string, activeAt time.Time) error {
	if r.item == nil || r.item.UserID != userID || r.item.SessionID != sessionID {
		return shared.ErrNotFound
	}
	for _, message := range []*conversation.Message{userMessage, assistantMessage} {
		message.ID = r.nextID
		r.nextID++
		copy := *message
		r.messages = append(r.messages, &copy)
	}
	r.item.Title = title
	r.item.TitleCustomized = titleCustomized
	return r.item.UpdateContext(summary, sources, activeAt)
}

func (r *publicAgentConversationRepo) FindTurnByClientID(_ context.Context, userID int64, sessionID, clientTurnID string) (*conversation.Message, *conversation.Message, error) {
	if r.item == nil || r.item.UserID != userID || r.item.SessionID != sessionID {
		return nil, nil, shared.ErrNotFound
	}
	var userMessage, assistantMessage *conversation.Message
	for _, message := range r.messages {
		if message.ClientTurnID != clientTurnID {
			continue
		}
		copy := *message
		if message.Role == conversation.RoleUser {
			userMessage = &copy
		} else if message.Role == conversation.RoleAssistant {
			assistantMessage = &copy
		}
	}
	if userMessage == nil {
		return nil, nil, conversation.ErrTurnNotFound
	}
	return userMessage, assistantMessage, nil
}

func (r *publicAgentConversationRepo) CommitTurn(ctx context.Context, userID int64, sessionID, title string, titleCustomized bool, userMessage, assistantMessage *conversation.Message, summary, sources string, activeAt time.Time, regenerate bool) error {
	storedUser, storedAssistant, err := r.FindTurnByClientID(ctx, userID, sessionID, userMessage.ClientTurnID)
	if err == nil {
		if storedUser.Content != userMessage.Content {
			return conversation.ErrTurnConflict
		}
		if storedAssistant != nil && storedAssistant.GenerationID == assistantMessage.GenerationID {
			*userMessage, *assistantMessage = *storedUser, *storedAssistant
			return nil
		}
		if storedAssistant != nil && !regenerate {
			return conversation.ErrTurnConflict
		}
		for index, message := range r.messages {
			if message.Role == conversation.RoleAssistant && message.ClientTurnID == userMessage.ClientTurnID {
				assistantMessage.ID = message.ID
				copy := *assistantMessage
				r.messages[index] = &copy
				*userMessage = *storedUser
				r.feedback = nil
				return r.item.UpdateContext(summary, sources, activeAt)
			}
		}
	}
	if err != nil && !errors.Is(err, conversation.ErrTurnNotFound) {
		return err
	}
	return r.RecordTurn(ctx, userID, sessionID, title, titleCustomized, userMessage, assistantMessage, summary, sources, activeAt)
}

func (r *publicAgentConversationRepo) SaveResponseFeedback(_ context.Context, feedback *conversation.ResponseFeedback) error {
	for _, message := range r.messages {
		if message.ID == feedback.MessageID && message.Role == conversation.RoleAssistant && message.RequestID == feedback.RequestID {
			copy := *feedback
			r.feedback = &copy
			return nil
		}
	}
	return conversation.ErrInvalidFeedback
}

func publicAgentRouter(client agentChatClient) *gin.Engine {
	repository := &publicAgentConversationRepo{}
	handler := newAgentHandler(client, conversationapp.NewService(repository), nil, nil)
	engine := gin.New()
	engine.Use(RequestIDMiddleware())
	engine.POST("/api/agent/chat", func(c *gin.Context) {
		c.Set(ContextUserID, int64(7))
		c.Set(ContextRole, "member")
		handler.Chat(c)
	})
	return engine
}

func performPublicAgentChat(engine *gin.Engine) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/agent/chat", strings.NewReader(`{"message":"你好","session_id":"session-7"}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer public-token")
	request.Header.Set("X-Request-ID", strings.Repeat("f", 32))
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func TestAgentChatUsesTrustedRequestIDWithoutLeakingAudit(t *testing.T) {
	client := &publicAgentClient{response: agentclient.ChatResponse{
		Answer:    "已完成",
		SessionID: "private-agent-session",
		RequestID: "private-request-id",
		Audit: &telemetry.AuditSummary{
			RequestID: "private-request-id",
			Telemetry: &telemetry.Snapshot{
				ModelCallCount: 1,
				ToolCallCount:  1,
				ModelCalls: []telemetry.ModelCall{{
					Sequence: 1,
					Model:    "private-model",
					Status:   "success",
				}},
				ToolCalls: []telemetry.ToolCall{{
					Sequence: 2,
					Name:     "private-tool",
					Status:   "success",
				}},
			},
		},
	}}
	response := performPublicAgentChat(publicAgentRouter(client))

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if len(client.requestIDs) != 1 {
		t.Fatalf("downstream call count = %d", len(client.requestIDs))
	}
	trustedID := response.Header().Get("X-Request-ID")
	if !requestIDPattern.MatchString(trustedID) || trustedID == strings.Repeat("f", 32) {
		t.Fatalf("trusted response ID = %q", trustedID)
	}
	if client.requestIDs[0] != trustedID {
		t.Fatalf("downstream ID = %q, response ID = %q", client.requestIDs[0], trustedID)
	}

	var envelope map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("decode public response: %v", err)
	}
	data, ok := envelope["data"].(map[string]any)
	if !ok {
		t.Fatalf("public data = %#v", envelope["data"])
	}
	for _, field := range []string{"audit", "telemetry", "model_calls", "tool_calls", "request_id"} {
		if _, exists := envelope[field]; exists {
			t.Fatalf("public envelope leaked %q: %s", field, response.Body.String())
		}
		if _, exists := data[field]; exists {
			t.Fatalf("public data leaked %q: %s", field, response.Body.String())
		}
	}
	for _, privateValue := range []string{"private-request-id", "private-model", "private-tool"} {
		if strings.Contains(response.Body.String(), privateValue) {
			t.Fatalf("public response leaked %q: %s", privateValue, response.Body.String())
		}
	}
}

func TestAgentChatMapsTimeoutsToGatewayTimeout(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "transport-deadline", err: context.DeadlineExceeded},
		{name: "private-agent-504", err: &agentclient.HTTPError{StatusCode: http.StatusGatewayTimeout, Message: "private timeout"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := &publicAgentClient{err: test.err}
			response := performPublicAgentChat(publicAgentRouter(client))

			if response.Code != http.StatusGatewayTimeout {
				t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusGatewayTimeout, response.Body.String())
			}
			var body Response
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != http.StatusGatewayTimeout || body.Message != "agent service timed out" {
				t.Fatalf("response = %+v", body)
			}
			if len(client.requestIDs) != 1 || client.requestIDs[0] != response.Header().Get("X-Request-ID") {
				t.Fatalf("request IDs = %v, response ID = %q", client.requestIDs, response.Header().Get("X-Request-ID"))
			}
			if strings.Contains(response.Body.String(), "private timeout") {
				t.Fatalf("private timeout detail leaked: %s", response.Body.String())
			}
		})
	}
}

func TestNewAgentHandlerNormalizesNilClient(t *testing.T) {
	handler := NewAgentHandler(nil, nil, nil, nil)
	engine := gin.New()
	engine.Use(RequestIDMiddleware())
	engine.POST("/api/agent/chat", handler.Chat)

	response := performPublicAgentChat(engine)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d: %s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	if requestID := response.Header().Get("X-Request-ID"); !requestIDPattern.MatchString(requestID) {
		t.Fatalf("X-Request-ID = %q", requestID)
	}
}
