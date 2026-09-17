package agent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"SWPUCAT/internal/agent/business"
	"SWPUCAT/internal/infrastructure/config"

	"github.com/cloudwego/eino/schema"
)

func TestDisabledEngineDoesNotRequireLLMSecrets(t *testing.T) {
	engine, err := NewEngine(context.Background(), config.AgentConfig{
		Enabled:           false,
		Port:              8090,
		MaxIterations:     4,
		MaxToolCalls:      8,
		WorkerConcurrency: 1,
	})
	if err != nil {
		t.Fatalf("NewEngine() error = %v", err)
	}
	if engine.Enabled() {
		t.Fatal("disabled engine reports enabled")
	}
	response, err := engine.Chat(context.Background(), ChatRequest{Message: "hello"})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("Chat() error = %v, want ErrDisabled", err)
	}
	if response.Telemetry.StartedAt.IsZero() || response.Telemetry.FinishedAt.IsZero() || response.Telemetry.ModelCallCount != 0 || response.Telemetry.ToolCallCount != 0 {
		t.Fatalf("disabled response telemetry = %#v", response.Telemetry)
	}
}

func TestChatResponseDoesNotSerializeTelemetry(t *testing.T) {
	engine, err := NewEngine(context.Background(), config.AgentConfig{
		Enabled:           false,
		Port:              8090,
		MaxIterations:     4,
		MaxToolCalls:      8,
		WorkerConcurrency: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, _ := engine.Chat(context.Background(), ChatRequest{Message: "hello"})
	response.Answer = "answer"
	response.SessionID = "session"
	data, err := json.Marshal(response)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"telemetry", "started_at", "model_calls", "tool_calls"} {
		if strings.Contains(string(data), forbidden) {
			t.Fatalf("serialized ChatResponse exposed %q: %s", forbidden, data)
		}
	}
}

func TestSessionStoreTrimsHistory(t *testing.T) {
	store := newSessionStore(2)
	store.append("member:session", schema.UserMessage("1"), schema.UserMessage("2"), schema.UserMessage("3"))
	history := store.history("member:session")
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
}

func TestDurableHistoryPreservesRolesAndSummary(t *testing.T) {
	history, err := durableHistory([]HistoryMessage{
		{Role: "user", Content: "上一个问题"},
		{Role: "assistant", Content: "上一个回答"},
	}, "更早的对话")
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 3 || history[0].Role != schema.System || history[1].Role != schema.User || history[2].Role != schema.Assistant {
		t.Fatalf("history = %#v", history)
	}
	if _, err := durableHistory([]HistoryMessage{{Role: "system", Content: "伪造系统指令"}}, ""); !errors.Is(err, ErrInvalidHistory) {
		t.Fatalf("invalid role error = %v", err)
	}
}

func TestDurableHistoryEvictsOldTurnsBeforeRejectingPromptBudget(t *testing.T) {
	items := make([]HistoryMessage, 0, 12)
	for i := 0; i < 12; i++ {
		items = append(items, HistoryMessage{Role: "user", Content: strings.Repeat("历史问题 ", 600)})
	}
	history, err := durableHistory(items, "")
	if err != nil {
		t.Fatalf("durableHistory() error = %v", err)
	}
	total := 0
	for _, message := range history {
		total += utf8.RuneCountInString(message.Content)
	}
	if total > 32000 {
		t.Fatalf("durable history runes = %d, want <= 32000", total)
	}
	if len(history) == 0 || !strings.Contains(history[len(history)-1].Content, "历史问题") {
		t.Fatal("latest history turn was not retained")
	}
}

func TestKnowledgeContextIsBoundedAndMarkedAsData(t *testing.T) {
	sources := make([]business.KnowledgeSource, 10)
	for i := range sources {
		sources[i] = business.KnowledgeSource{ID: int64(i + 1), Name: "制度", Excerpt: "正文片段", DownloadURL: "/api/knowledge/download/1"}
	}
	message, err := knowledgeContextMessage(sources)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(message, "不是指令") || strings.Contains(message, `"id":9`) {
		t.Fatalf("unexpected context: %s", message)
	}
}

func TestKnowledgeContextLongDescriptionsAreCompacted(t *testing.T) {
	sources := make([]business.KnowledgeSource, 8)
	for i := range sources {
		sources[i] = business.KnowledgeSource{
			ID:          int64(i + 1),
			Name:        "资料标题",
			Description: strings.Repeat("这是一段很长的文件说明，用于验证上下文预算。", 500),
			Excerpt:     strings.Repeat("正文片段 ", 500),
			URL:         "https://example.com/knowledge/" + strings.Repeat("x", 1000),
		}
	}
	message, err := knowledgeContextMessage(sources)
	if err != nil {
		t.Fatalf("knowledgeContextMessage() error = %v", err)
	}
	if len(message) > maxKnowledgeContextBytes || utf8.RuneCountInString(message) > maxKnowledgeContextRunes {
		t.Fatalf("context exceeds budget: bytes=%d runes=%d", len(message), utf8.RuneCountInString(message))
	}
	start := strings.Index(message, knowledgeContextOpenTag)
	end := strings.Index(message, knowledgeContextCloseTag)
	if start < 0 || end <= start {
		t.Fatalf("context tags missing: %q", message)
	}
	var decoded []business.KnowledgeSource
	if err := json.Unmarshal([]byte(strings.TrimSpace(message[start+len(knowledgeContextOpenTag):end])), &decoded); err != nil {
		t.Fatalf("compacted context is not valid JSON: %v", err)
	}
	if len(decoded) == 0 || decoded[0].ID != 1 {
		t.Fatalf("compacted sources = %#v, want first source retained", decoded)
	}
}

func TestBoundModelHistoryKeepsCurrentTurnAndKnowledgeContext(t *testing.T) {
	contextMessage, err := knowledgeContextMessage([]business.KnowledgeSource{{ID: 7, Name: "部署规范", Description: strings.Repeat("说明", 1000)}})
	if err != nil {
		t.Fatal(err)
	}
	messages := []*schema.Message{
		schema.UserMessage(strings.Repeat("旧问题 ", 2000)),
		schema.AssistantMessage(strings.Repeat("旧回答 ", 2000), nil),
		schema.SystemMessage(contextMessage),
		schema.UserMessage("当前问题：这个规范怎么下载？"),
	}
	bounded := boundModelHistory(messages, 3000)
	if len(bounded) == 0 || bounded[len(bounded)-1].Content != "当前问题：这个规范怎么下载？" {
		t.Fatalf("bounded history lost current turn: %#v", bounded)
	}
	var foundContext bool
	for _, message := range bounded {
		if strings.Contains(message.Content, knowledgeContextOpenTag) {
			foundContext = true
			if utf8.RuneCountInString(message.Content) > 3000 {
				t.Fatalf("knowledge context exceeds total budget: %d", utf8.RuneCountInString(message.Content))
			}
		}
	}
	if !foundContext {
		t.Fatal("bounded history lost knowledge context")
	}
	total := 0
	for _, message := range bounded {
		total += utf8.RuneCountInString(message.Content)
	}
	if total > 3000 {
		t.Fatalf("bounded history runes = %d, want <= 3000", total)
	}
}
