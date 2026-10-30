package agentaudit

import (
	"context"
	"time"
)

// Repository appends a complete run aggregate. Implementations must persist
// the parent run and every tool call atomically and must never overwrite an
// existing request identifier.
type Repository interface {
	CreateRun(ctx context.Context, run *Run) error
}

// ReadFilter bounds administrator analytics queries. The application layer
// validates all fields before passing a filter to storage.
type ReadFilter struct {
	From     time.Time
	To       time.Time
	Status   RunStatus
	Page     int
	PageSize int
}

// ReadSummary contains only aggregate counters. It never contains prompts,
// answers, model payloads, authorization data, or raw tool arguments.
type ReadSummary struct {
	TotalRuns          int64
	TotalDurationMS    int64
	RunsWithDuration   int64
	PromptTokens       int64
	CompletionTokens   int64
	TotalTokens        int64
	CachedTokens       int64
	RunsWithTokenUsage int64
	StatusCounts       map[string]int64
	ToolCounts         map[string]int64
}

// Reader exposes privacy-safe administrator reporting over append-only audit
// aggregates. Implementations must apply the complete date/status filter to
// list, count, summary, and tool aggregation queries.
type Reader interface {
	ListRuns(ctx context.Context, filter ReadFilter) ([]Run, int64, error)
	FindRun(ctx context.Context, id int64) (*Run, error)
	SummarizeRuns(ctx context.Context, filter ReadFilter) (ReadSummary, error)
}
