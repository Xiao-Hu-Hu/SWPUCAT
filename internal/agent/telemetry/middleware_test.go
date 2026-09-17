package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"SWPUCAT/internal/agent/business"

	componentopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	goopenai "github.com/meguminnnnnnnnn/go-openai"
)

type fakeChatModel struct {
	generateFn func(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error)
	streamFn   func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error)
}

var _ model.BaseChatModel = (*fakeChatModel)(nil)

func (f *fakeChatModel) Generate(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.Message, error) {
	if f.generateFn == nil {
		return nil, errors.New("unexpected Generate call")
	}
	return f.generateFn(ctx, input, opts...)
}

func (f *fakeChatModel) Stream(ctx context.Context, input []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if f.streamFn == nil {
		return nil, errors.New("unexpected Stream call")
	}
	return f.streamFn(ctx, input, opts...)
}

func TestModelGenerateTelemetry(t *testing.T) {
	toolCalls := []schema.ToolCall{{ID: "SECRET_CALL_ID_913", Function: schema.FunctionCall{Name: "SECRET_TOOL_NAME_913"}}}
	for i := 0; i < maxToolRequestsPerModelCall+3; i++ {
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:       fmt.Sprintf("call-%d", i),
			Function: schema.FunctionCall{Name: "search_knowledge", Arguments: `{"query":"SECRET_ARGUMENT_913"}`},
		})
	}
	response := &schema.Message{
		Role:             schema.Assistant,
		Content:          "SECRET_ANSWER_913",
		ReasoningContent: "SECRET_REASONING_913",
		ToolCalls:        toolCalls,
		ResponseMeta: &schema.ResponseMeta{
			FinishReason: "tool_calls",
			Usage: &schema.TokenUsage{
				PromptTokens:     12,
				CompletionTokens: 5,
				TotalTokens:      17,
				PromptTokenDetails: schema.PromptTokenDetails{
					CachedTokens: 4,
				},
				CompletionTokensDetails: schema.CompletionTokensDetails{
					ReasoningTokens: 3,
				},
			},
		},
	}
	base := &fakeChatModel{generateFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
		return response, nil
	}}
	middleware := NewMiddleware("openai_compatible", "test-model")
	wrapped, err := middleware.WrapModel(context.Background(), base, &adk.ModelContext{Tools: []*schema.ToolInfo{
		{Name: "search_knowledge"},
		{Name: "get_checkin_status"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	collector := NewCollector()
	ctx := WithCollector(context.Background(), collector)
	got, err := wrapped.Generate(ctx, []*schema.Message{
		schema.SystemMessage("SECRET_SYSTEM_913"),
		schema.UserMessage("SECRET_INPUT_913"),
	})
	if err != nil || got != response {
		t.Fatalf("Generate() = (%#v, %v), want original response", got, err)
	}

	snapshot := collector.Snapshot()
	if len(snapshot.ModelCalls) != 1 {
		t.Fatalf("model calls = %d, want 1", len(snapshot.ModelCalls))
	}
	call := snapshot.ModelCalls[0]
	if call.Provider != "openai_compatible" || call.Model != "test-model" || call.InputMessages != 2 || call.ToolsOffered != 2 || call.Status != "success" || call.FinishReason != "tool_calls" {
		t.Fatalf("unexpected model call: %#v", call)
	}
	wantUsage := &TokenUsage{PromptTokens: 12, CompletionTokens: 5, TotalTokens: 17, CachedTokens: 4, ReasoningTokens: 3}
	if call.Usage == nil || *call.Usage != *wantUsage {
		t.Fatalf("usage = %#v, want %#v", call.Usage, wantUsage)
	}
	if len(call.ToolRequests) != maxToolRequestsPerModelCall || !call.ToolRequestsTruncated {
		t.Fatalf("tool requests = %d truncated=%v", len(call.ToolRequests), call.ToolRequestsTruncated)
	}
	if call.ToolRequests[0].Name != "[unknown]" || call.ToolRequests[0].CallID != "" {
		t.Fatalf("unknown tool request was not anonymized: %#v", call.ToolRequests[0])
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"SECRET_ANSWER_913", "SECRET_REASONING_913", "SECRET_SYSTEM_913", "SECRET_INPUT_913", "SECRET_ARGUMENT_913", "SECRET_TOOL_NAME_913", "SECRET_CALL_ID_913"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("model telemetry leaked %q: %s", secret, data)
		}
	}

	// Without a request collector the wrapper must be behaviorally transparent.
	plain, err := wrapped.Generate(context.Background(), nil)
	if err != nil || plain != response {
		t.Fatalf("Generate without collector = (%#v, %v)", plain, err)
	}
}

func TestModelToolRequestsRemainDistinct(t *testing.T) {
	tests := []struct {
		name     string
		firstID  string
		secondID string
		wantID   string
	}{
		{name: "invalid ids", firstID: strings.Repeat("a", maxOpaqueIDBytes+1), secondID: strings.Repeat("b", maxOpaqueIDBytes+1)},
		{name: "duplicate valid id", firstID: "duplicate-call-id", secondID: "duplicate-call-id", wantID: "call-1"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
				{ID: test.firstID, Function: schema.FunctionCall{Name: "clock_in"}},
				{ID: test.secondID, Function: schema.FunctionCall{Name: "clock_in"}},
			}}
			base := &fakeChatModel{generateFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
				return response, nil
			}}
			wrapper := wrapTestModel(t, base, "clock_in")
			collector := NewCollector()
			if _, err := wrapper.Generate(WithCollector(context.Background(), collector), nil); err != nil {
				t.Fatal(err)
			}
			requests := collector.Snapshot().ModelCalls[0].ToolRequests
			if len(requests) != 2 || requests[0].CallID != test.wantID || requests[1].CallID != test.wantID {
				t.Fatalf("independent requests collapsed or leaked: %#v", requests)
			}
		})
	}
}

func TestModelStreamTelemetry(t *testing.T) {
	t.Run("merges chunks", func(t *testing.T) {
		index := 0
		chunks := []*schema.Message{
			{
				Role:             schema.Assistant,
				Content:          "SECRET_STREAM_CONTENT_913",
				ReasoningContent: "SECRET_STREAM_REASONING_913",
				ToolCalls:        []schema.ToolCall{{Index: &index, ID: "call-1"}},
				ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{
					PromptTokens:       9,
					PromptTokenDetails: schema.PromptTokenDetails{CachedTokens: 5},
				}},
			},
			{
				Role:      schema.Assistant,
				ToolCalls: []schema.ToolCall{{Index: &index, Function: schema.FunctionCall{Name: "search_knowledge", Arguments: `{"query":"SECRET_STREAM_ARGUMENT_913"}`}}},
				ResponseMeta: &schema.ResponseMeta{
					FinishReason: "stop",
					Usage: &schema.TokenUsage{
						CompletionTokens:        4,
						TotalTokens:             13,
						CompletionTokensDetails: schema.CompletionTokensDetails{ReasoningTokens: 2},
					},
				},
			},
		}
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return schema.StreamReaderFromArray(chunks), nil
		}}
		wrapped := wrapTestModel(t, base, "search_knowledge")
		collector := NewCollector()
		reader, err := wrapped.Stream(WithCollector(context.Background(), collector), []*schema.Message{schema.UserMessage("private")})
		if err != nil {
			t.Fatal(err)
		}
		consumeStream(t, reader)
		snapshot := collector.Snapshot()
		if len(snapshot.ModelCalls) != 1 {
			t.Fatalf("model calls = %d, want 1", len(snapshot.ModelCalls))
		}
		call := snapshot.ModelCalls[0]
		if call.Status != "success" || call.FinishReason != "stop" || len(call.ToolRequests) != 1 || call.ToolRequests[0] != (ToolRequest{CallID: "call-1", Name: "search_knowledge"}) {
			t.Fatalf("unexpected stream call: %#v", call)
		}
		want := TokenUsage{PromptTokens: 9, CompletionTokens: 4, TotalTokens: 13, CachedTokens: 5, ReasoningTokens: 2}
		if call.Usage == nil || *call.Usage != want {
			t.Fatalf("stream usage = %#v, want %#v", call.Usage, want)
		}
		data, _ := json.Marshal(snapshot)
		for _, secret := range []string{"SECRET_STREAM_CONTENT_913", "SECRET_STREAM_REASONING_913", "SECRET_STREAM_ARGUMENT_913"} {
			if strings.Contains(string(data), secret) {
				t.Fatalf("stream telemetry leaked %q: %s", secret, data)
			}
		}
	})

	t.Run("invalid ids remain distinct", func(t *testing.T) {
		firstIndex, secondIndex := 0, 1
		chunks := []*schema.Message{{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{Index: &firstIndex, ID: strings.Repeat("a", maxOpaqueIDBytes+1), Function: schema.FunctionCall{Name: "clock_in"}},
			{Index: &secondIndex, ID: strings.Repeat("b", maxOpaqueIDBytes+1), Function: schema.FunctionCall{Name: "clock_in"}},
		}}}
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return schema.StreamReaderFromArray(chunks), nil
		}}
		wrapped := wrapTestModel(t, base, "clock_in")
		collector := NewCollector()
		reader, err := wrapped.Stream(WithCollector(context.Background(), collector), nil)
		if err != nil {
			t.Fatal(err)
		}
		consumeStream(t, reader)
		requests := collector.Snapshot().ModelCalls[0].ToolRequests
		if len(requests) != 2 || requests[0].CallID != "" || requests[1].CallID != "" {
			t.Fatalf("stream invalid IDs collapsed or leaked: %#v", requests)
		}
	})

	t.Run("duplicate valid ids remain distinct", func(t *testing.T) {
		firstIndex, secondIndex := 0, 1
		chunks := []*schema.Message{{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
			{Index: &firstIndex, ID: "duplicate-call-id", Function: schema.FunctionCall{Name: "clock_in"}},
			{Index: &secondIndex, ID: "duplicate-call-id", Function: schema.FunctionCall{Name: "clock_in"}},
		}}}
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return schema.StreamReaderFromArray(chunks), nil
		}}
		wrapped := wrapTestModel(t, base, "clock_in")
		collector := NewCollector()
		reader, err := wrapped.Stream(WithCollector(context.Background(), collector), nil)
		if err != nil {
			t.Fatal(err)
		}
		consumeStream(t, reader)
		requests := collector.Snapshot().ModelCalls[0].ToolRequests
		if len(requests) != 2 || requests[0].CallID != "call-1" || requests[1].CallID != "call-1" {
			t.Fatalf("stream requests sharing an ID collapsed: %#v", requests)
		}
	})

	t.Run("initial error", func(t *testing.T) {
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return nil, context.DeadlineExceeded
		}}
		assertStreamStartStatus(t, base, "timeout")
	})

	t.Run("nil reader", func(t *testing.T) {
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return nil, nil
		}}
		assertStreamStartStatus(t, base, "error")
	})

	t.Run("receive error is recorded before delivery", func(t *testing.T) {
		terminal := errors.New("SECRET_STREAM_ERROR_913")
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return streamWithError([]*schema.Message{{Role: schema.Assistant, ResponseMeta: &schema.ResponseMeta{FinishReason: "length"}}}, terminal), nil
		}}
		wrapped := wrapTestModel(t, base)
		collector := NewCollector()
		reader, err := wrapped.Stream(WithCollector(context.Background(), collector), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		if _, err := reader.Recv(); err != nil {
			t.Fatalf("first Recv() error = %v", err)
		}
		if _, err := reader.Recv(); !errors.Is(err, terminal) {
			t.Fatalf("terminal error = %v, want %v", err, terminal)
		}
		snapshot := collector.Snapshot()
		if len(snapshot.ModelCalls) != 1 || snapshot.ModelCalls[0].Status != "error" || snapshot.ModelCalls[0].FinishReason != "length" {
			t.Fatalf("stream error snapshot = %#v", snapshot.ModelCalls)
		}
		data, _ := json.Marshal(snapshot)
		if strings.Contains(string(data), "SECRET_STREAM_ERROR_913") {
			t.Fatalf("stream error leaked: %s", data)
		}
	})

	t.Run("consumer closes early", func(t *testing.T) {
		chunks := make([]*schema.Message, 20)
		for i := range chunks {
			chunks[i] = &schema.Message{Role: schema.Assistant, Content: "chunk"}
		}
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return schema.StreamReaderFromArray(chunks), nil
		}}
		wrapped := wrapTestModel(t, base)
		collector := NewCollector()
		reader, err := wrapped.Stream(WithCollector(context.Background(), collector), nil)
		if err != nil {
			t.Fatal(err)
		}
		reader.Close()
		call := waitForModelCall(t, collector)
		if call.Status != "canceled" || call.Error == nil || call.Error.Code != "downstream_closed" {
			t.Fatalf("early-close call = %#v", call)
		}
	})

	t.Run("synchronous panic is rethrown", func(t *testing.T) {
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			panic("SECRET_STREAM_PANIC_913")
		}}
		wrapped := wrapTestModel(t, base)
		collector := NewCollector()
		var recovered any
		func() {
			defer func() { recovered = recover() }()
			_, _ = wrapped.Stream(WithCollector(context.Background(), collector), nil)
		}()
		if recovered == nil {
			t.Fatal("Stream panic was not rethrown")
		}
		call := collector.Snapshot().ModelCalls[0]
		if call.Status != "panic" || call.Error == nil || strings.Contains(call.Error.Message, "SECRET_STREAM_PANIC_913") {
			t.Fatalf("panic telemetry = %#v", call)
		}
	})

	t.Run("reader and close panics are contained", func(t *testing.T) {
		base := &fakeChatModel{streamFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
			return &schema.StreamReader[*schema.Message]{}, nil
		}}
		wrapped := wrapTestModel(t, base)
		collector := NewCollector()
		reader, err := wrapped.Stream(WithCollector(context.Background(), collector), nil)
		if err != nil {
			t.Fatal(err)
		}
		defer reader.Close()
		if _, err := reader.Recv(); err == nil {
			t.Fatal("panic stream returned no terminal error")
		}
		call := waitForModelCall(t, collector)
		if call.Status != "panic" || call.Error == nil || call.Error.Code != "panic" {
			t.Fatalf("panic stream telemetry = %#v", call)
		}
	})
}

func TestRequestLocalIdentifierAliases(t *testing.T) {
	firstRawID := "sk-proj-LEAK913"
	secondRawID := "eyJhbGciOiJIUzI1NiJ9.LEAK913.signature"
	response := &schema.Message{Role: schema.Assistant, ToolCalls: []schema.ToolCall{
		{ID: firstRawID, Function: schema.FunctionCall{Name: "clock_in"}},
		{ID: secondRawID, Function: schema.FunctionCall{Name: "clock_out"}},
	}}
	base := &fakeChatModel{generateFn: func(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
		return response, nil
	}}
	wrapped := wrapTestModel(t, base, "clock_in", "clock_out")
	collector := NewCollector()
	ctx := WithCollector(context.Background(), collector)
	if _, err := wrapped.Generate(ctx, nil); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		id   string
	}{
		{name: "clock_in", id: firstRawID},
		{name: "clock_out", id: secondRawID},
	} {
		toolCall := wrapTestTool(t, NewMiddleware("openai_compatible", "test-model"), test.name, test.id, func(context.Context, string, ...tool.Option) (string, error) {
			return `{"type":"in","date":"2026-09-18"}`, nil
		})
		if _, err := toolCall(ctx, `{}`); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := collector.Snapshot()
	if len(snapshot.ModelCalls) != 1 || len(snapshot.ModelCalls[0].ToolRequests) != 2 || len(snapshot.ToolCalls) != 2 {
		t.Fatalf("alias snapshot counts = %#v", snapshot)
	}
	if snapshot.ModelCalls[0].ToolRequests[0].CallID != "call-1" || snapshot.ToolCalls[0].CallID != "call-1" ||
		snapshot.ModelCalls[0].ToolRequests[1].CallID != "call-2" || snapshot.ToolCalls[1].CallID != "call-2" {
		t.Fatalf("request-local aliases did not correlate: model=%#v tools=%#v", snapshot.ModelCalls[0].ToolRequests, snapshot.ToolCalls)
	}
	data, _ := json.Marshal(snapshot)
	for _, raw := range []string{firstRawID, secondRawID} {
		if strings.Contains(string(data), raw) {
			t.Fatalf("raw call ID leaked %q: %s", raw, data)
		}
	}
}

func TestToolMiddlewareTelemetry(t *testing.T) {
	middleware := NewMiddleware("provider", "model")

	t.Run("success", func(t *testing.T) {
		wrapped := wrapTestTool(t, middleware, "clock_in", "call-1", func(context.Context, string, ...tool.Option) (string, error) {
			return `{"id":91,"type":"in","date":"2026-09-18","time":"08:30:00","private":"SECRET_RESULT_913"}`, nil
		})
		collector := NewCollector()
		result, err := wrapped(WithCollector(context.Background(), collector), `{}`)
		if err != nil || !strings.Contains(result, "SECRET_RESULT_913") {
			t.Fatalf("wrapped tool = (%q, %v)", result, err)
		}
		call := collector.Snapshot().ToolCalls[0]
		if call.Status != "success" || call.Name != "clock_in" || call.CallID != "call-1" || call.Error != nil ||
			strings.Contains(call.Result.JSON, "SECRET_RESULT_913") || call.Result.JSON != `{"date":"2026-09-18","type":"in"}` {
			t.Fatalf("tool success telemetry = %#v", call)
		}
	})

	t.Run("empty result", func(t *testing.T) {
		wrapped := wrapTestTool(t, middleware, "clock_out", "call-2", func(context.Context, string, ...tool.Option) (string, error) {
			return "", nil
		})
		collector := NewCollector()
		_, _ = wrapped(WithCollector(context.Background(), collector), `{}`)
		call := collector.Snapshot().ToolCalls[0]
		if call.Result.JSON != "null" || call.Status != "success" {
			t.Fatalf("empty result telemetry = %#v", call)
		}
	})

	t.Run("limit error", func(t *testing.T) {
		wrapped := wrapTestTool(t, middleware, "search_knowledge", "call-3", func(context.Context, string, ...tool.Option) (string, error) {
			return "", business.ErrToolCallLimit
		})
		collector := NewCollector()
		_, err := wrapped(WithCollector(context.Background(), collector), `{"query":"SECRET_QUERY_913"}`)
		if !errors.Is(err, business.ErrToolCallLimit) {
			t.Fatalf("tool error = %v", err)
		}
		call := collector.Snapshot().ToolCalls[0]
		if call.Status != "error" || call.Error == nil || call.Error.Category != "limit" || strings.Contains(call.Arguments.JSON, "SECRET_QUERY_913") {
			t.Fatalf("limit telemetry = %#v", call)
		}
	})

	t.Run("panic is recorded and rethrown", func(t *testing.T) {
		wrapped := wrapTestTool(t, middleware, "clock_in", "call-4", func(context.Context, string, ...tool.Option) (string, error) {
			panic("SECRET_TOOL_PANIC_913")
		})
		collector := NewCollector()
		var recovered any
		func() {
			defer func() { recovered = recover() }()
			_, _ = wrapped(WithCollector(context.Background(), collector), `{}`)
		}()
		if recovered == nil {
			t.Fatal("tool panic was not rethrown")
		}
		call := collector.Snapshot().ToolCalls[0]
		if call.Status != "panic" || call.Error == nil || strings.Contains(call.Error.Message, "SECRET_TOOL_PANIC_913") {
			t.Fatalf("panic telemetry = %#v", call)
		}
	})

	t.Run("no collector is transparent", func(t *testing.T) {
		wrapped := wrapTestTool(t, middleware, "clock_in", "call-5", func(_ context.Context, arguments string, _ ...tool.Option) (string, error) {
			return arguments, nil
		})
		result, err := wrapped(context.Background(), `{"value":1}`)
		if err != nil || result != `{"value":1}` {
			t.Fatalf("transparent call = (%q, %v)", result, err)
		}
	})

	t.Run("concurrent calls", func(t *testing.T) {
		wrapped := wrapTestTool(t, middleware, "clock_in", "call-concurrent", func(context.Context, string, ...tool.Option) (string, error) {
			return `{"status":"ok"}`, nil
		})
		collector := NewCollector()
		ctx := WithCollector(context.Background(), collector)
		const count = 100
		var wg sync.WaitGroup
		errs := make(chan error, count)
		wg.Add(count)
		for i := 0; i < count; i++ {
			go func() {
				defer wg.Done()
				_, err := wrapped(ctx, `{}`)
				errs <- err
			}()
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent tool error = %v", err)
			}
		}
		snapshot := collector.Snapshot()
		if snapshot.ToolCallCount != count || len(snapshot.ToolCalls) != count || snapshot.DroppedToolCalls != 0 {
			t.Fatalf("concurrent counts = total %d recorded %d dropped %d", snapshot.ToolCallCount, len(snapshot.ToolCalls), snapshot.DroppedToolCalls)
		}
		seen := make(map[uint64]struct{}, count)
		for _, call := range snapshot.ToolCalls {
			if _, exists := seen[call.Sequence]; exists {
				t.Fatalf("duplicate sequence %d", call.Sequence)
			}
			seen[call.Sequence] = struct{}{}
		}
	})
}

func TestErrorClassification(t *testing.T) {
	parameter := "messages[0].content"
	tests := []struct {
		name      string
		err       error
		category  string
		code      string
		typeName  string
		status    int
		retryable bool
	}{
		{name: "canceled", err: context.Canceled, category: "canceled", code: "context_canceled"},
		{name: "deadline", err: context.DeadlineExceeded, category: "timeout", code: "deadline_exceeded", retryable: true},
		{name: "tool limit", err: business.ErrToolCallLimit, category: "limit", code: "tool_call_limit"},
		{name: "auth", err: &componentopenai.APIError{HTTPStatusCode: 401, Code: "invalid_api_key", Type: "authentication_error", Param: &parameter, Message: "SECRET_PROVIDER_MESSAGE_913"}, category: "provider_auth", code: "invalid_api_key", typeName: "authentication_error", status: 401},
		{name: "rate limit", err: &componentopenai.APIError{HTTPStatusCode: 429, Code: "rate_limit", Type: "rate_limit_error"}, category: "provider_rate_limit", code: "rate_limit", typeName: "rate_limit_error", status: 429, retryable: true},
		{name: "quota exhausted", err: &componentopenai.APIError{HTTPStatusCode: 429, Code: "insufficient_quota", Type: "rate_limit_error"}, category: "provider_rate_limit", code: "insufficient_quota", typeName: "rate_limit_error", status: 429},
		{name: "overloaded", err: &componentopenai.APIError{HTTPStatusCode: 503, Code: "model_overloaded", Type: "overloaded_error"}, category: "provider_unavailable", code: "model_overloaded", typeName: "overloaded_error", status: 503, retryable: true},
		{name: "server", err: &componentopenai.APIError{HTTPStatusCode: 503, Code: "unavailable"}, category: "provider_unavailable", code: "provider_unavailable", status: 503, retryable: true},
		{name: "invalid status", err: &componentopenai.APIError{HTTPStatusCode: 13800138000, Code: "SECRET_STATUS_CODE_913"}, category: "provider", code: "provider_error"},
		{name: "go openai api", err: &goopenai.APIError{HTTPStatusCode: 500, Code: "server_error", Type: "server_error", Message: "SECRET_GO_API_913"}, category: "provider_unavailable", code: "server_error", typeName: "server_error", status: 500, retryable: true},
		{name: "request error", err: &goopenai.RequestError{HTTPStatusCode: 502, Err: errors.New("SECRET_REQUEST_ERR_913"), Body: []byte("SECRET_REQUEST_BODY_913")}, category: "provider_unavailable", code: "provider_request_error", status: 502, retryable: true},
		{name: "network timeout", err: fakeNetworkError{timeout: true}, category: "timeout", code: "network_timeout", retryable: true},
		{name: "execution", err: errors.New("SECRET_EXECUTION_913"), category: "execution", code: "execution_error"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			info := classifyError(test.err)
			if info == nil || info.Category != test.category || info.Code != test.code || info.Type != test.typeName || info.HTTPStatus != test.status || info.Retryable != test.retryable {
				t.Fatalf("classifyError(%T) = %#v", test.err, info)
			}
			data, _ := json.Marshal(info)
			for _, secret := range []string{"SECRET_PROVIDER_MESSAGE_913", "SECRET_GO_API_913", "SECRET_REQUEST_ERR_913", "SECRET_REQUEST_BODY_913", "SECRET_EXECUTION_913", "SECRET_STATUS_CODE_913"} {
				if strings.Contains(string(data), secret) {
					t.Fatalf("error telemetry leaked %q: %s", secret, data)
				}
			}
		})
	}

	sensitive := &componentopenai.APIError{
		HTTPStatusCode: 400,
		Code:           "api_key=SECRET_CODE_913",
		Type:           "token_SECRET_TYPE_913",
		Param:          stringPointer("password_SECRET_PARAM_913"),
	}
	info := classifyError(sensitive)
	if info.Code != "provider_request" || info.Type != "" || info.Param != "" {
		t.Fatalf("sensitive provider metadata was retained: %#v", info)
	}
	if panicInfo := panicError("SECRET_PANIC_913"); strings.Contains(panicInfo.Message, "SECRET_PANIC_913") {
		t.Fatalf("panic metadata leaked: %#v", panicInfo)
	}
}

func TestOpenAIProviderResponseID(t *testing.T) {
	rawResponseID := "sk-proj-PROVIDER-ID-LEAK913"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{
			"id":"`+rawResponseID+`",
			"object":"chat.completion",
			"created":1,
			"model":"test-model",
			"choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":2,"completion_tokens":1,"total_tokens":3}
		}`)
	}))
	defer server.Close()

	base, err := componentopenai.NewChatModel(context.Background(), &componentopenai.ChatModelConfig{
		APIKey:  "test-key",
		Model:   "test-model",
		BaseURL: server.URL + "/v1",
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	wrapped := wrapTestModel(t, base)
	collector := NewCollector()
	if _, err := wrapped.Generate(WithCollector(context.Background(), collector), []*schema.Message{schema.UserMessage("hello")}); err != nil {
		t.Fatal(err)
	}
	snapshot := collector.Snapshot()
	call := snapshot.ModelCalls[0]
	if call.ProviderResponseAlias != "response-1" || call.FinishReason != "stop" {
		t.Fatalf("provider metadata = %#v", call)
	}
	data, _ := json.Marshal(snapshot)
	if strings.Contains(string(data), rawResponseID) {
		t.Fatalf("raw provider response ID leaked: %s", data)
	}
}

func wrapTestModel(t *testing.T, base model.BaseChatModel, toolNames ...string) model.BaseChatModel {
	t.Helper()
	infos := make([]*schema.ToolInfo, 0, len(toolNames))
	for _, name := range toolNames {
		infos = append(infos, &schema.ToolInfo{Name: name})
	}
	wrapped, err := NewMiddleware("provider", "model").WrapModel(context.Background(), base, &adk.ModelContext{Tools: infos})
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func wrapTestTool(t *testing.T, middleware *Middleware, name, callID string, endpoint adk.InvokableToolCallEndpoint) adk.InvokableToolCallEndpoint {
	t.Helper()
	wrapped, err := middleware.WrapInvokableToolCall(context.Background(), endpoint, &adk.ToolContext{Name: name, CallID: callID})
	if err != nil {
		t.Fatal(err)
	}
	return wrapped
}

func consumeStream(t *testing.T, reader *schema.StreamReader[*schema.Message]) {
	t.Helper()
	defer reader.Close()
	for {
		_, err := reader.Recv()
		if errors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			t.Fatalf("Recv() error = %v", err)
		}
	}
}

func streamWithError(messages []*schema.Message, terminal error) *schema.StreamReader[*schema.Message] {
	reader, writer := schema.Pipe[*schema.Message](len(messages) + 1)
	for _, message := range messages {
		writer.Send(message, nil)
	}
	writer.Send(nil, terminal)
	writer.Close()
	return reader
}

func assertStreamStartStatus(t *testing.T, base model.BaseChatModel, wantStatus string) {
	t.Helper()
	wrapped := wrapTestModel(t, base)
	collector := NewCollector()
	reader, err := wrapped.Stream(WithCollector(context.Background(), collector), nil)
	if err == nil || reader != nil {
		t.Fatalf("Stream() = (%#v, %v), want nil/error", reader, err)
	}
	snapshot := collector.Snapshot()
	if len(snapshot.ModelCalls) != 1 || snapshot.ModelCalls[0].Status != wantStatus {
		t.Fatalf("stream start snapshot = %#v, want status %q", snapshot.ModelCalls, wantStatus)
	}
}

func waitForModelCall(t *testing.T, collector *Collector) ModelCall {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := collector.Snapshot()
		if len(snapshot.ModelCalls) > 0 {
			return snapshot.ModelCalls[0]
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for model telemetry")
	return ModelCall{}
}

type fakeNetworkError struct {
	timeout bool
}

func (e fakeNetworkError) Error() string   { return "SECRET_NETWORK_ERROR_913" }
func (e fakeNetworkError) Timeout() bool   { return e.timeout }
func (e fakeNetworkError) Temporary() bool { return true }

func stringPointer(value string) *string { return &value }
