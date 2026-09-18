package agentaudit

import "context"

// Repository appends a complete run aggregate. Implementations must persist
// the parent run and every tool call atomically and must never overwrite an
// existing request identifier.
type Repository interface {
	CreateRun(ctx context.Context, run *Run) error
}
