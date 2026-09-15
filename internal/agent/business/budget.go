package business

import (
	"context"
	"errors"
	"sync"
)

var ErrToolCallLimit = errors.New("agent tool call limit exceeded")

type toolCallBudgetKey struct{}

type toolCallBudget struct {
	mu        sync.Mutex
	remaining int
}

// WithToolCallBudget attaches a per-request tool budget. A non-positive
// budget means that tools are disabled for that request; callers should pass
// the validated AgentConfig value.
func WithToolCallBudget(ctx context.Context, max int) context.Context {
	return context.WithValue(ctx, toolCallBudgetKey{}, &toolCallBudget{remaining: max})
}

func ConsumeToolCall(ctx context.Context) error {
	budget, _ := ctx.Value(toolCallBudgetKey{}).(*toolCallBudget)
	if budget == nil {
		return nil
	}
	budget.mu.Lock()
	defer budget.mu.Unlock()
	if budget.remaining <= 0 {
		return ErrToolCallLimit
	}
	budget.remaining--
	return nil
}
