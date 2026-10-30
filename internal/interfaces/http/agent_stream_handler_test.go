package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	conversationapp "SWPUCAT/internal/application/conversation"
	"SWPUCAT/internal/agent/confirmation"
	"SWPUCAT/internal/infrastructure/agentclient"

	"github.com/gin-gonic/gin"
)

const (
	streamClientTurnID  = "11111111111111111111111111111111"
	streamGenerationID1 = "22222222222222222222222222222222"
	streamGenerationID2 = "33333333333333333333333333333333"
)

type publicStreamingAgentClient struct {
	mu          sync.Mutex
	streamCalls int
	responses   []agentclient.ChatResponse
	deltas      [][]string
	stream      func(context.Context, agentclient.ChatStreamCallback) (agentclient.ChatResponse, error)
}

func (c *publicStreamingAgentClient) Chat(context.Context, agentclient.ChatRequest, string, string) (agentclient.ChatResponse, error) {
	return agentclient.ChatResponse{}, errors.New("unexpected non-streaming call")
}

func (c *publicStreamingAgentClient) ChatStream(ctx context.Context, _ agentclient.ChatRequest, _, requestID string, callback agentclient.ChatStreamCallback) (agentclient.ChatResponse, error) {
	c.mu.Lock()
	call := c.streamCalls
	c.streamCalls++
	custom := c.stream
	c.mu.Unlock()
	if custom != nil {
		return custom(ctx, callback)
	}
	if call >= len(c.responses) {
		return agentclient.ChatResponse{}, errors.New("unexpected stream call")
	}
	for _, delta := range c.deltas[call] {
		if err := callback(agentclient.ChatStreamDelta{Delta: delta}); err != nil {
			return agentclient.ChatResponse{}, err
		}
	}
	result := c.responses[call]
	result.RequestID = requestID
	return result, nil
}

func publicAgentStreamRouter(client agentChatClient, repo *publicAgentConversationRepo) *gin.Engine {
	gin.SetMode(gin.TestMode)
	handler := newAgentHandler(client, conversationapp.NewService(repo), nil, nil)
	engine := gin.New()
	engine.Use(RequestIDMiddleware())
	engine.POST("/api/agent/chat/stream", func(c *gin.Context) {
		c.Set(ContextUserID, int64(7))
		c.Set(ContextRole, "member")
		handler.ChatStream(c)
	})
	return engine
}

func performPublicAgentStream(engine *gin.Engine, generationID string, regenerate bool) *httptest.ResponseRecorder {
	body := map[string]any{
		"message": "查询本周工时", "session_id": "session-7",
		"client_turn_id": streamClientTurnID, "generation_id": generationID,
	}
	if regenerate {
		body["regenerate"] = true
	}
	payload, _ := json.Marshal(body)
	request := httptest.NewRequest(http.MethodPost, "/api/agent/chat/stream", strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer token")
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	return response
}

func TestAgentChatStreamPersistsOnlyTerminalTurnAndReplaysCompletedGeneration(t *testing.T) {
	repo := &publicAgentConversationRepo{}
	client := &publicStreamingAgentClient{
		responses: []agentclient.ChatResponse{{Answer: "本周已完成 8 小时", SessionID: "private"}},
		deltas:    [][]string{{"本周已完成", " 8 小时"}},
	}
	engine := publicAgentStreamRouter(client, repo)

	first := performPublicAgentStream(engine, streamGenerationID1, false)
	if first.Code != http.StatusOK {
		t.Fatalf("first status = %d body=%s", first.Code, first.Body.String())
	}
	body := first.Body.String()
	if strings.Index(body, `"type":"delta"`) < 0 || strings.Index(body, `"type":"done"`) <= strings.Index(body, `"type":"delta"`) {
		t.Fatalf("stream ordering = %s", body)
	}
	if len(repo.messages) != 2 || repo.messages[0].ClientTurnID != streamClientTurnID || repo.messages[1].GenerationID != streamGenerationID1 {
		t.Fatalf("persisted messages = %+v", repo.messages)
	}
	if !strings.Contains(body, `"conversation_persisted":true`) || !strings.Contains(body, `"message_id":`) {
		t.Fatalf("terminal persistence metadata missing: %s", body)
	}

	replay := performPublicAgentStream(engine, streamGenerationID1, false)
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body.String(), `"cached":true`) {
		t.Fatalf("replay response = %d %s", replay.Code, replay.Body.String())
	}
	if client.streamCalls != 1 || len(repo.messages) != 2 {
		t.Fatalf("replay invoked model or duplicated messages: calls=%d messages=%d", client.streamCalls, len(repo.messages))
	}
}

func TestAgentChatStreamRegenerationReplacesAssistantWithoutDuplicatingUser(t *testing.T) {
	repo := &publicAgentConversationRepo{}
	client := &publicStreamingAgentClient{
		responses: []agentclient.ChatResponse{
			{Answer: "旧回答", SessionID: "private"},
			{Answer: "新回答", SessionID: "private"},
		},
		deltas: [][]string{{"旧回答"}, {"新回答"}},
	}
	engine := publicAgentStreamRouter(client, repo)
	performPublicAgentStream(engine, streamGenerationID1, false)
	oldAssistantID := repo.messages[1].ID
	repo.feedback = nil

	response := performPublicAgentStream(engine, streamGenerationID2, true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"answer":"新回答"`) {
		t.Fatalf("regeneration response = %d %s", response.Code, response.Body.String())
	}
	if len(repo.messages) != 2 || repo.messages[1].ID != oldAssistantID || repo.messages[1].Content != "新回答" || repo.messages[1].GenerationID != streamGenerationID2 {
		t.Fatalf("regeneration did not replace assistant in place: %+v", repo.messages)
	}
	if client.streamCalls != 2 {
		t.Fatalf("stream calls = %d, want 2", client.streamCalls)
	}
}

func TestAgentChatStreamKeepsLiveConfirmationTokenInDoneEvent(t *testing.T) {
	repo := &publicAgentConversationRepo{}
	const signedToken = "signed-confirmation-token-abc123"
	client := &publicStreamingAgentClient{
		responses: []agentclient.ChatResponse{{
			Answer:    "已生成待确认卡片，业务操作尚未执行。请核对卡片信息并点击“确认执行”；最终结果以确认后服务端返回的结构化结果卡片为准。",
			SessionID: "private",
			Proposals: []confirmation.Proposal{{
				Tool: "generate_invitation_code", Title: "生成邀请码", Summary: "确认后将生成一个 10 分钟有效的邀请码",
				Token: signedToken, ExpiresAt: "2026-09-18T00:00:00Z",
			}},
		}},
		deltas: [][]string{{"已生成待确认卡片"}},
	}
	engine := publicAgentStreamRouter(client, repo)

	response := performPublicAgentStream(engine, streamGenerationID1, false)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	body := response.Body.String()

	// The streaming handler rebuilds the done event from the persisted turn when
	// persistence succeeds. The persisted actions_json strips confirmation
	// tokens for privacy, so the rebuild previously lost the token and left the
	// browser with a disabled "确认执行" card. The done event must carry the live
	// token while still attaching the DB-issued message IDs.
	var done agentPublicStreamEnvelope
	found := false
	for _, line := range strings.Split(body, "\n") {
		if strings.TrimSpace(line) == "" || !strings.Contains(line, `"type":"done"`) {
			continue
		}
		if err := json.Unmarshal([]byte(line), &done); err != nil {
			t.Fatalf("parse done event: %v", err)
		}
		found = true
		break
	}
	if !found || done.Data == nil {
		t.Fatalf("done event missing: %q", body)
	}
	if len(done.Data.Proposals) != 1 || done.Data.Proposals[0].Token != signedToken {
		t.Fatalf("done proposal token = %+v, want live token %q", done.Data.Proposals, signedToken)
	}
	if done.Data.MessageID == 0 || done.Data.UserMessageID == 0 {
		t.Fatalf("persisted message IDs not attached to done event: %+v", *done.Data)
	}
	if len(repo.messages) != 2 {
		t.Fatalf("persisted messages = %d, want 2", len(repo.messages))
	}
	if persisted := repo.messages[1].ActionsJSON; strings.Contains(persisted, signedToken) || strings.Contains(persisted, `"token"`) {
		t.Fatalf("persisted actions_json leaked confirmation token: %s", persisted)
	}
}

func TestAgentChatStreamCancellationPropagatesAndDoesNotPersistPartialOutput(t *testing.T) {
	repo := &publicAgentConversationRepo{}
	deltaWritten := make(chan struct{})
	clientObservedCancel := make(chan struct{})
	client := &publicStreamingAgentClient{}
	client.stream = func(ctx context.Context, callback agentclient.ChatStreamCallback) (agentclient.ChatResponse, error) {
		if err := callback(agentclient.ChatStreamDelta{Delta: "partial"}); err != nil {
			return agentclient.ChatResponse{}, err
		}
		close(deltaWritten)
		<-ctx.Done()
		close(clientObservedCancel)
		return agentclient.ChatResponse{}, ctx.Err()
	}
	engine := publicAgentStreamRouter(client, repo)
	body := `{"message":"查询本周工时","session_id":"session-7","client_turn_id":"` + streamClientTurnID + `","generation_id":"` + streamGenerationID1 + `"}`
	ctx, cancel := context.WithCancel(context.Background())
	request := httptest.NewRequest(http.MethodPost, "/api/agent/chat/stream", strings.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		engine.ServeHTTP(response, request)
		close(done)
	}()
	<-deltaWritten
	cancel()
	<-done
	<-clientObservedCancel
	if len(repo.messages) != 0 {
		t.Fatalf("canceled request persisted partial messages: %+v", repo.messages)
	}
	if !strings.Contains(response.Body.String(), `"delta":"partial"`) || strings.Contains(response.Body.String(), `"type":"done"`) {
		t.Fatalf("canceled stream body = %s", response.Body.String())
	}
}
