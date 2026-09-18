package agentclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T: %v", err, err)
	}
	if clientErr.Kind != KindDownstream || clientErr.FailureCode != FailureCodeLegacy || clientErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("client error = %+v", clientErr)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected HTTPError 503, got %v", err)
	}
	if httpErr.Kind != KindDownstream || httpErr.FailureCode != FailureCodeLegacy {
		t.Fatalf("HTTP error classification = %+v", httpErr)
	}
	if httpErr.RequestID != testRequestID || httpErr.Audit != nil {
		t.Fatalf("legacy HTTP error metadata = request_id %q audit %#v", httpErr.RequestID, httpErr.Audit)
	}
}

func TestChatUsesStableDownstreamFailureCodesWithoutParsingMessages(t *testing.T) {
	tests := []struct {
		name        string
		failureCode FailureCode
		message     string
	}{
		{name: "disabled", failureCode: FailureCodeAgentDisabled, message: "arbitrary disabled wording"},
		{name: "busy", failureCode: FailureCodeAgentBusy, message: "arbitrary busy wording"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"code":503,"message":"` + tt.message + `","failure_code":"` + string(tt.failureCode) + `"}`))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
			var clientErr *ClientError
			if !errors.As(err, &clientErr) {
				t.Fatalf("expected ClientError, got %T: %v", err, err)
			}
			if clientErr.Kind != KindDownstream || clientErr.FailureCode != tt.failureCode || clientErr.StatusCode != http.StatusServiceUnavailable || clientErr.Message != tt.message {
				t.Fatalf("client error = %+v", clientErr)
			}
		})
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
	var clientErr *ClientError
	if !errors.As(err, &clientErr) || clientErr.Kind != KindProtocol || clientErr.FailureCode != FailureCodeProtocol || clientErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("mismatch client error = %T %+v", err, err)
	}
	if clientErr.Audit != nil {
		t.Fatalf("mismatched audit was retained by client error: %+v", clientErr.Audit)
	}
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusBadGateway || httpErr.Message != "invalid agent response" {
		t.Fatalf("mismatch error = %T %+v", err, err)
	}
	if httpErr.RequestID != testRequestID || httpErr.Audit != nil {
		t.Fatalf("mismatched audit was associated with trusted request: %+v", httpErr)
	}
}

func TestChatRejectsUnknownFailureCodeAsProtocolError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{
			"code":503,
			"message":"do not parse this message",
			"failure_code":"attacker_controlled_code",
			"audit":{"request_id":"` + testRequestID + `","telemetry":{"duration_ms":23}}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T: %v", err, err)
	}
	if clientErr.Kind != KindProtocol || clientErr.FailureCode != FailureCodeProtocol || clientErr.StatusCode != http.StatusBadGateway || clientErr.Message != "invalid agent response" {
		t.Fatalf("unknown code classification = %+v", clientErr)
	}
	if clientErr.Audit == nil || clientErr.Audit.Telemetry == nil || clientErr.Audit.Telemetry.DurationMS != 23 {
		t.Fatalf("trusted audit was not retained: %+v", clientErr.Audit)
	}
	if strings.Contains(err.Error(), "attacker_controlled_code") {
		t.Fatalf("unknown failure code leaked through error: %v", err)
	}
}

func TestChatRejectsFailureCodeOnSuccessfulEnvelope(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{
			"code":0,
			"message":"success",
			"failure_code":"agent_busy",
			"data":{"answer":"must not be accepted","session_id":"s1"},
			"audit":{"request_id":"` + testRequestID + `","telemetry":{"duration_ms":29}}
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T: %v", err, err)
	}
	if clientErr.Kind != KindProtocol || clientErr.FailureCode != FailureCodeProtocol || clientErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("success failure-code classification = %+v", clientErr)
	}
	if clientErr.Audit == nil || clientErr.Audit.Telemetry == nil || clientErr.Audit.Telemetry.DurationMS != 29 {
		t.Fatalf("trusted audit was not retained: %+v", clientErr.Audit)
	}
}

func TestChatRejectsFailureCodeStatusMismatchAsProtocolError(t *testing.T) {
	tests := []struct {
		name         string
		httpStatus   int
		envelopeCode int
		failureCode  FailureCode
	}{
		{name: "failure code contradicts status", httpStatus: http.StatusServiceUnavailable, envelopeCode: http.StatusServiceUnavailable, failureCode: FailureCodeInvalidMessage},
		{name: "envelope code contradicts status", httpStatus: http.StatusBadRequest, envelopeCode: http.StatusServiceUnavailable, failureCode: FailureCodeInvalidMessage},
		{name: "failure envelope uses success status", httpStatus: http.StatusOK, envelopeCode: http.StatusBadRequest, failureCode: FailureCodeInvalidMessage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.httpStatus)
				_, _ = fmt.Fprintf(w, `{"code":%d,"message":"misleading","failure_code":"%s","audit":{"request_id":"%s","telemetry":{"duration_ms":37}}}`, tt.envelopeCode, tt.failureCode, testRequestID)
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
			var clientErr *ClientError
			if !errors.As(err, &clientErr) {
				t.Fatalf("expected ClientError, got %T: %v", err, err)
			}
			if clientErr.Kind != KindProtocol || clientErr.FailureCode != FailureCodeProtocol || clientErr.StatusCode != http.StatusBadGateway {
				t.Fatalf("mismatch classification = %+v", clientErr)
			}
			if clientErr.Audit == nil || clientErr.Audit.Telemetry == nil || clientErr.Audit.Telemetry.DurationMS != 37 {
				t.Fatalf("trusted audit was not retained: %+v", clientErr.Audit)
			}
		})
	}
}

func TestChatProtocolFailuresRetainTrustedAudit(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{name: "missing data"},
		{name: "invalid data", data: `"not an object"`},
		{name: "incomplete data", data: `{"answer":"","session_id":"s1"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataField := ""
			if tt.data != "" {
				dataField = `,"data":` + tt.data
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"code":0,"message":"success"` + dataField + `,"audit":{"request_id":"` + testRequestID + `","telemetry":{"duration_ms":31}}}`))
			}))
			defer server.Close()

			client, err := NewClient(server.URL, "", time.Second)
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
			var clientErr *ClientError
			if !errors.As(err, &clientErr) {
				t.Fatalf("expected ClientError, got %T: %v", err, err)
			}
			if clientErr.Kind != KindProtocol || clientErr.FailureCode != FailureCodeProtocol || clientErr.StatusCode != http.StatusBadGateway {
				t.Fatalf("protocol classification = %+v", clientErr)
			}
			if clientErr.Audit == nil || clientErr.Audit.Telemetry == nil || clientErr.Audit.Telemetry.DurationMS != 31 {
				t.Fatalf("trusted audit was not retained: %+v", clientErr.Audit)
			}
		})
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
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T: %v", err, err)
	}
	if clientErr.Kind != KindTimeout || clientErr.FailureCode != FailureCodeExecutionTimeout || clientErr.StatusCode != 0 || clientErr.RequestID != testRequestID {
		t.Fatalf("timeout classification = %+v", clientErr)
	}
	var netErr net.Error
	if !errors.Is(err, context.DeadlineExceeded) && (!errors.As(err, &netErr) || !netErr.Timeout()) {
		t.Fatalf("timeout classification was lost: %T %v", err, err)
	}
}

func TestChatCanceledContextRemainsDetectable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"answer":"unexpected","session_id":"s1"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Chat(ctx, ChatRequest{Message: "hello"}, "", testRequestID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was lost: %T %v", err, err)
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T: %v", err, err)
	}
	if clientErr.Kind != KindCanceled || clientErr.FailureCode != FailureCodeRequestCanceled || clientErr.StatusCode != 0 || clientErr.RequestID != testRequestID {
		t.Fatalf("cancellation classification = %+v", clientErr)
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		t.Fatalf("transport cancellation unexpectedly classified as HTTP error: %+v", httpErr)
	}
}

func TestChatDeadlineContextRemainsDetectable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"answer":"unexpected","session_id":"s1"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	_, err = client.Chat(ctx, ChatRequest{Message: "hello"}, "", testRequestID)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline was lost: %T %v", err, err)
	}
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T: %v", err, err)
	}
	if clientErr.Kind != KindTimeout || clientErr.FailureCode != FailureCodeExecutionTimeout || clientErr.StatusCode != 0 || clientErr.RequestID != testRequestID {
		t.Fatalf("deadline classification = %+v", clientErr)
	}
}

func TestChatTransportFailureIsTyped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	serverURL := server.URL
	server.Close()

	client, err := NewClient(serverURL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Chat(context.Background(), ChatRequest{Message: "hello"}, "", testRequestID)
	var clientErr *ClientError
	if !errors.As(err, &clientErr) {
		t.Fatalf("expected ClientError, got %T: %v", err, err)
	}
	if clientErr.Kind != KindTransport || clientErr.FailureCode != FailureCodeTransport || clientErr.StatusCode != 0 || clientErr.RequestID != testRequestID {
		t.Fatalf("transport classification = %+v", clientErr)
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
