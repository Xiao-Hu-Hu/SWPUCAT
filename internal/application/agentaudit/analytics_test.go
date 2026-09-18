package agentauditapp

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "SWPUCAT/internal/domain/agentaudit"
)

type fakeAnalyticsReader struct {
	runs    []domain.Run
	total   int64
	detail  *domain.Run
	summary domain.ReadSummary
	filter  domain.ReadFilter
	err     error
}

func (r *fakeAnalyticsReader) ListRuns(_ context.Context, filter domain.ReadFilter) ([]domain.Run, int64, error) {
	r.filter = filter
	return r.runs, r.total, r.err
}

func (r *fakeAnalyticsReader) FindRun(_ context.Context, _ int64) (*domain.Run, error) {
	return r.detail, r.err
}

func (r *fakeAnalyticsReader) SummarizeRuns(_ context.Context, filter domain.ReadFilter) (domain.ReadSummary, error) {
	r.filter = filter
	return r.summary, r.err
}

func TestAnalyticsListAndCostEstimate(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	prompt, completion, total, cached := int64(1_000_000), int64(500_000), int64(1_500_000), int64(200_000)
	repo := &fakeAnalyticsReader{total: 1, runs: []domain.Run{{
		ID: 9, RequestID: "0123456789abcdef0123456789abcdef", Status: domain.RunStatusSuccess,
		TelemetryStatus: domain.TelemetryCaptured, DurationMS: 1500, PromptTokens: &prompt,
		CompletionTokens: &completion, TotalTokens: &total, CachedTokens: &cached, CreatedAt: now,
	}}}
	service := NewAnalyticsService(repo, Pricing{Enabled: true, InputUSDPerMillion: 1, CachedInputUSDPerMillion: 0.25, OutputUSDPerMillion: 2})
	service.now = func() time.Time { return now }

	page, err := service.List(context.Background(), AnalyticsQuery{})
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 || !page.Items[0].EstimatedCost.Known || page.Items[0].EstimatedCost.MicroUSD != 1_850_000 {
		t.Fatalf("page = %#v", page)
	}
	if !repo.filter.From.Equal(now.Add(-7*24*time.Hour)) || !repo.filter.To.Equal(now) || repo.filter.Page != 1 || repo.filter.PageSize != 20 {
		t.Fatalf("default filter = %#v", repo.filter)
	}
}

func TestAnalyticsSummaryRequiresCompleteUsageForTotalCost(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	repo := &fakeAnalyticsReader{summary: domain.ReadSummary{
		TotalRuns: 2, TotalDurationMS: 300, RunsWithDuration: 2,
		PromptTokens: 100, CompletionTokens: 50, TotalTokens: 150, RunsWithTokenUsage: 1,
		StatusCounts: map[string]int64{"success": 1, "timeout": 1}, ToolCounts: map[string]int64{"search_knowledge": 2},
	}}
	service := NewAnalyticsService(repo, Pricing{Enabled: true, InputUSDPerMillion: 1, OutputUSDPerMillion: 2})
	service.now = func() time.Time { return now }

	summary, err := service.Summary(context.Background(), AnalyticsQuery{})
	if err != nil {
		t.Fatalf("Summary() error = %v", err)
	}
	if summary.SuccessRate != 0.5 || summary.AverageDurationMS != 150 || summary.EstimatedCost.Known || summary.CostKnownRuns != 0 {
		t.Fatalf("summary = %#v", summary)
	}
}

func TestAnalyticsValidationAndUnavailableReader(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	service := NewAnalyticsService(&fakeAnalyticsReader{}, Pricing{})
	service.now = func() time.Time { return now }
	_, err := service.List(context.Background(), AnalyticsQuery{From: now.Add(-91 * 24 * time.Hour), To: now})
	if !errors.Is(err, ErrInvalidAnalyticsQuery) {
		t.Fatalf("range error = %v", err)
	}
	_, err = service.List(context.Background(), AnalyticsQuery{Status: "invented"})
	if !errors.Is(err, ErrInvalidAnalyticsQuery) {
		t.Fatalf("status error = %v", err)
	}
	_, err = NewAnalyticsService(nil, Pricing{}).List(context.Background(), AnalyticsQuery{})
	if !errors.Is(err, ErrAnalyticsUnavailable) {
		t.Fatalf("nil reader error = %v", err)
	}
}
