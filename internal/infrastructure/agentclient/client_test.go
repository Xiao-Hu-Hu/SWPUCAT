package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testRequestID = "0123456789abcdef0123456789abcdef"

func TestChatForwardsIdentityAndDecodesEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("X-Request-ID"); got != "req-1" {
			t.Fatalf("request id = %q", got)
		}
		if got := r.Header.Get("X-Agent-Internal-Secret"); got != "secret" {
			t.Fatalf("internal secret = %q", got)
		}
		var request ChatRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if !request.HistoryProvided || len(request.History) != 1 || len(request.KnowledgeContext) != 1 {
			t.Fatalf("durable context was not forwarded: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"answer":"已完成","session_id":"s1","sources":[{"id":3,"name":"值班流程","category_name":"流程"}]}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Chat(context.Background(), ChatRequest{
		Message: "打卡", HistoryProvided: true,
		History:          []HistoryMessage{{Role: "user", Content: "之前的问题"}},
		KnowledgeContext: []KnowledgeSource{{ID: 3, Name: "值班流程", Excerpt: "正文"}},
	}, "Bearer token", "req-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer != "已完成" || result.SessionID != "s1" {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.Sources) != 1 || result.Sources[0].ID != 3 {
		t.Fatalf("sources were not decoded: %+v", result.Sources)
	}
	if result.RequestID != "req-1" || result.Audit != nil {
		t.Fatalf("legacy response metadata = request_id %q audit %#v", result.RequestID, result.Audit)
	}
}

func TestChatReturnsHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":503,"message":"agent is disabled"}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTPError 503, got %v", err)
	}
	if httpErr.RequestID != testRequestID || httpErr.Audit != nil {
		t.Fatalf("legacy HTTP error metadata = request_id %q audit %#v", httpErr.RequestID, httpErr.Audit)
	}
}

func TestChatDecodesAuditWithoutSerializingItPublicly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code":0,
			"message":"success",
			"data":{"answer":"ok","session_id":"session-1"},
			"audit":{
				"request_id":"` + testRequestID + `",
				"telemetry":{
					"duration_ms":17,
					"model_call_count":1,
					"tool_call_count":1,
					"usage":{"reported_model_calls":1,"missing_model_calls":0,"reported_totals":{"total_tokens":9}},
					"model_calls":[],
					"tool_calls":[]
				}
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	if err != nil {
		t.Fatal(err)
	}
	if result.RequestID != testRequestID || result.Audit == nil || result.Audit.Telemetry == nil {
		t.Fatalf("audit metadata was not retained: %+v", result)
	}
	if result.Audit.Telemetry.DurationMS != 17 || result.Audit.Telemetry.ModelCallCount != 1 || result.Audit.Telemetry.ToolCallCount != 1 {
		t.Fatalf("unexpected telemetry: %+v", result.Audit.Telemetry)
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"request_id", "audit", "telemetry", "model_calls", "tool_calls"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("public ChatResponse JSON leaks %q: %s", forbidden, encoded)
		}
	}
}

func TestChatHTTPErrorRetainsTrustedAudit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte(`{
			"code":504,
			"message":"agent execution timed out",
			"audit":{
				"request_id":"` + testRequestID + `",
				"telemetry":{
					"duration_ms":1000,
					"model_call_count":1,
					"tool_call_count":0,
					"usage":{"reported_model_calls":0,"missing_model_calls":1,"reported_totals":{}},
					"model_calls":[],
					"tool_calls":[]
				}
			}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %T: %v", err, err)
	}
	if httpErr.StatusCode != http.StatusGatewayTimeout || httpErr.RequestID != testRequestID {
		t.Fatalf("unexpected HTTP error: %+v", httpErr)
	}
	if httpErr.Audit == nil || httpErr.Audit.Telemetry == nil || httpErr.Audit.Telemetry.DurationMS != 1000 {
		t.Fatalf("error audit was not retained: %+v", httpErr.Audit)
	}
}

func TestChatRejectsMismatchedAuditRequestID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"code":0,
			"message":"success",
			"data":{"answer":"ok","session_id":"session-1"},
			"audit":{"request_id":"ffffffffffffffffffffffffffffffff"}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	if err == nil {
		t.Fatalf("expected request id mismatch, got result=%+v", result)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadGateway || httpErr.Message != "invalid agent response" {
		t.Fatalf("mismatch error = %T %+v", err, err)
	}
	if httpErr.RequestID != testRequestID || httpErr.Audit != nil {
		t.Fatalf("mismatched audit was associated with trusted request: %+v", httpErr)
	}
}

func TestChatTransportTimeoutRemainsDetectable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"answer":"late","session_id":"s1"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	if err == nil {
		t.Fatal("expected transport timeout")
	}
	var netErr net.Error
	if !errors.Is(err, context.DeadlineExceeded) && (!errors.As(err, &netErr) || !netErr.Timeout()) {
		t.Fatalf("timeout classification was lost: %T %v", err, err)
	}
}

func TestChatRejectsOversizedResponseWithoutTrustingPartialAudit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"answer":"ok","session_id":"s1"}}`))
		_, _ = w.Write([]byte(strings.Repeat(" ", maxAgentResponseBytes)))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadGateway || httpErr.Message != "agent response is too large" {
		t.Fatalf("oversized response error = %T %+v", err, err)
	}
	if httpErr.RequestID != testRequestID || httpErr.Audit != nil {
		t.Fatalf("oversized response metadata = %+v", httpErr)
	}
}
