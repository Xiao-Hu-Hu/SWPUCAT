package agent

import (
	"context"
	"errors"
	"testing"

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
	_, err = engine.Chat(context.Background(), ChatRequest{Message: "hello"})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("Chat() error = %v, want ErrDisabled", err)
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
