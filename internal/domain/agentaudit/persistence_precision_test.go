package agentaudit

import (
	"errors"
	"testing"
	"time"
)

func TestRunValidateAllowsPostgresTimestampPrecisionDrift(t *testing.T) {
	// The original elapsed time is 999.8 microseconds, so its whole-millisecond
	// duration is zero. PostgreSQL rounds each timestamptz endpoint to a
	// microsecond; for these fractions the loaded endpoints become 1ms apart.
	base := time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)
	startedAt := base.Add(400 * time.Nanosecond)
	finishedAt := startedAt.Add(time.Millisecond - 200*time.Nanosecond)
	run, err := NewRun(RunMetadata{
		RequestID:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status:          RunStatusSuccess,
		TelemetryStatus: TelemetryStatusUnavailable,
		StartedAt:       startedAt,
		FinishedAt:      finishedAt,
		CreatedAt:       base.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}
	if run.DurationMS != 0 {
		t.Fatalf("original DurationMS = %d, want 0", run.DurationMS)
	}

	run.StartedAt = startedAt.Round(time.Microsecond)
	run.FinishedAt = finishedAt.Round(time.Microsecond)
	if got := run.FinishedAt.Sub(run.StartedAt).Milliseconds(); got != 1 {
		t.Fatalf("simulated PostgreSQL duration = %dms, want 1ms", got)
	}
	if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("unpersisted one-millisecond mismatch error = %v, want ErrInvalidRun", err)
	}
	run.ID = 1
	if err := run.Validate(); err != nil {
		t.Fatalf("one-millisecond persistence drift rejected: %v", err)
	}
}

func TestRunValidateRejectsDurationDriftBeyondPostgresPrecision(t *testing.T) {
	base := time.Date(2026, 9, 18, 2, 0, 0, 0, time.UTC)
	run, err := NewRun(RunMetadata{
		RequestID:       "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Status:          RunStatusSuccess,
		TelemetryStatus: TelemetryStatusUnavailable,
		StartedAt:       base,
		FinishedAt:      base.Add(time.Millisecond),
		CreatedAt:       base.Add(time.Second),
	})
	if err != nil {
		t.Fatalf("NewRun() error = %v", err)
	}

	run.ID = 1
	run.DurationMS += 2
	if err := run.Validate(); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("two-millisecond duration drift error = %v, want ErrInvalidRun", err)
	}
}
