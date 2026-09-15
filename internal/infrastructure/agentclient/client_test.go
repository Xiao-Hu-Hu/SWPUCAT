package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
	_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", "")
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTPError 503, got %v", err)
	}
}
