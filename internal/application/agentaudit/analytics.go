package agentauditapp

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"

	domain "SWPUCAT/internal/domain/agentaudit"
)

var (
	ErrAnalyticsUnavailable  = errors.New("agent audit analytics unavailable")
	ErrInvalidAnalyticsQuery = errors.New("invalid agent audit analytics query")
)

const maxAnalyticsRange = 90 * 24 * time.Hour

// Pricing describes provider prices in USD per one million tokens. Prices are
// configuration, not persisted business facts; every response reports whether
// estimation was enabled so an unknown cost can never be mistaken for zero.
type Pricing struct {
	Enabled                  bool
	InputUSDPerMillion       float64
	CachedInputUSDPerMillion float64
	OutputUSDPerMillion      float64
}

func (p Pricing) Valid() bool {
	return finiteNonNegative(p.InputUSDPerMillion) && finiteNonNegative(p.CachedInputUSDPerMillion) && finiteNonNegative(p.OutputUSDPerMillion)
}

type AnalyticsQuery struct {
	From     time.Time
	To       time.Time
	Status   string
	Page     int
	PageSize int
}

type CostEstimate struct {
	Known    bool  `json:"known"`
	MicroUSD int64 `json:"micro_usd,omitempty"`
}

type RunListItem struct {
	ID               int64        `json:"id"`
	RequestID        string       `json:"request_id"`
	UserID           *int64       `json:"user_id,omitempty"`
	SessionID        *string      `json:"session_id,omitempty"`
	ActorRole        *string      `json:"actor_role,omitempty"`
	Status           string       `json:"status"`
	TelemetryStatus  string       `json:"telemetry_status"`
	HTTPStatus       *int         `json:"http_status,omitempty"`
	FailureCategory  *string      `json:"failure_category,omitempty"`
	FailureCode      *string      `json:"failure_code,omitempty"`
	DurationMS       int64        `json:"duration_ms"`
	ModelCallCount   *int         `json:"model_call_count,omitempty"`
	ToolCallCount    *int         `json:"tool_call_count,omitempty"`
	PromptTokens     *int64       `json:"prompt_tokens,omitempty"`
	CompletionTokens *int64       `json:"completion_tokens,omitempty"`
	TotalTokens      *int64       `json:"total_tokens,omitempty"`
	CachedTokens     *int64       `json:"cached_tokens,omitempty"`
	EstimatedCost    CostEstimate `json:"estimated_cost"`
	CreatedAt        time.Time    `json:"created_at"`
}

type ToolCallDTO struct {
	Sequence             int64   `json:"sequence"`
	Name                 string  `json:"name"`
	IsWrite              bool    `json:"is_write"`
	Status               string  `json:"status"`
	DurationMS           int64   `json:"duration_ms"`
	ArgumentsJSON        string  `json:"arguments_json"`
	ResultJSON           string  `json:"result_json"`
	ArgumentsRedacted    bool    `json:"arguments_redacted"`
	ResultRedacted       bool    `json:"result_redacted"`
	ErrorCategory        *string `json:"error_category,omitempty"`
	ErrorCode            *string `json:"error_code,omitempty"`
	BusinessObjectType   *string `json:"business_object_type,omitempty"`
	BusinessObjectID     *int64  `json:"business_object_id,omitempty"`
	BusinessObjectStatus *string `json:"business_object_status,omitempty"`
}

type RunDetail struct {
	RunListItem
	ToolCalls []ToolCallDTO `json:"tool_calls"`
}

type RunPage struct {
	Items    []RunListItem `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

type AnalyticsSummary struct {
	From               time.Time        `json:"from"`
	To                 time.Time        `json:"to"`
	TotalRuns          int64            `json:"total_runs"`
	SuccessRuns        int64            `json:"success_runs"`
	SuccessRate        float64          `json:"success_rate"`
	AverageDurationMS  float64          `json:"average_duration_ms"`
	PromptTokens       int64            `json:"prompt_tokens"`
	CompletionTokens   int64            `json:"completion_tokens"`
	TotalTokens        int64            `json:"total_tokens"`
	CachedTokens       int64            `json:"cached_tokens"`
	RunsWithTokenUsage int64            `json:"runs_with_token_usage"`
	EstimatedCost      CostEstimate     `json:"estimated_cost"`
	CostKnownRuns      int64            `json:"cost_known_runs"`
	StatusCounts       map[string]int64 `json:"status_counts"`
	ToolCounts         map[string]int64 `json:"tool_counts"`
}

type AnalyticsService struct {
	reader  domain.Reader
	pricing Pricing
	now     func() time.Time
}

func NewAnalyticsService(reader domain.Reader, pricing Pricing) *AnalyticsService {
	return &AnalyticsService{reader: reader, pricing: pricing, now: time.Now}
}

func (s *AnalyticsService) List(ctx context.Context, query AnalyticsQuery) (*RunPage, error) {
	filter, err := s.filter(query)
	if err != nil {
		return nil, err
	}
	runs, total, err := s.reader.ListRuns(ctx, filter)
	if err != nil {
		return nil, err
	}
	items := make([]RunListItem, 0, len(runs))
	for i := range runs {
		items = append(items, s.listItem(&runs[i]))
	}
	return &RunPage{Items: items, Total: total, Page: filter.Page, PageSize: filter.PageSize}, nil
}

func (s *AnalyticsService) Detail(ctx context.Context, id int64) (*RunDetail, error) {
	if s == nil || s.reader == nil {
		return nil, ErrAnalyticsUnavailable
	}
	if id <= 0 {
		return nil, ErrInvalidAnalyticsQuery
	}
	run, err := s.reader.FindRun(ctx, id)
	if err != nil {
		return nil, err
	}
	result := &RunDetail{RunListItem: s.listItem(run), ToolCalls: make([]ToolCallDTO, 0, len(run.ToolCalls))}
	for _, call := range run.ToolCalls {
		result.ToolCalls = append(result.ToolCalls, ToolCallDTO{
			Sequence: call.Sequence, Name: call.Name, IsWrite: call.IsWrite, Status: call.Status,
			DurationMS: call.DurationMS, ArgumentsJSON: call.ArgumentsJSON, ResultJSON: call.ResultJSON,
			ArgumentsRedacted: call.ArgumentsRedacted, ResultRedacted: call.ResultRedacted,
			ErrorCategory: call.ErrorCategory, ErrorCode: call.ErrorCode,
			BusinessObjectType: call.BusinessObjectType, BusinessObjectID: call.BusinessObjectID,
			BusinessObjectStatus: call.BusinessObjectStatus,
		})
	}
	return result, nil
}

func (s *AnalyticsService) Summary(ctx context.Context, query AnalyticsQuery) (*AnalyticsSummary, error) {
	filter, err := s.filter(query)
	if err != nil {
		return nil, err
	}
	summary, err := s.reader.SummarizeRuns(ctx, filter)
	if err != nil {
		return nil, err
	}
	result := &AnalyticsSummary{
		From: filter.From, To: filter.To, TotalRuns: summary.TotalRuns,
		PromptTokens: summary.PromptTokens, CompletionTokens: summary.CompletionTokens,
		TotalTokens: summary.TotalTokens, CachedTokens: summary.CachedTokens,
		RunsWithTokenUsage: summary.RunsWithTokenUsage,
		StatusCounts:       cloneCounts(summary.StatusCounts), ToolCounts: cloneCounts(summary.ToolCounts),
	}
	result.SuccessRuns = result.StatusCounts[string(domain.RunStatusSuccess)]
	if result.TotalRuns > 0 {
		result.SuccessRate = float64(result.SuccessRuns) / float64(result.TotalRuns)
	}
	if summary.RunsWithDuration > 0 {
		result.AverageDurationMS = float64(summary.TotalDurationMS) / float64(summary.RunsWithDuration)
	}
	if s.pricing.Enabled && s.pricing.Valid() && summary.RunsWithTokenUsage == summary.TotalRuns && summary.TotalRuns > 0 {
		prompt, completion, cached := summary.PromptTokens, summary.CompletionTokens, summary.CachedTokens
		result.EstimatedCost = estimateCost(s.pricing, &prompt, &completion, &cached)
		result.CostKnownRuns = summary.RunsWithTokenUsage
	}
	return result, nil
}

func (s *AnalyticsService) filter(query AnalyticsQuery) (domain.ReadFilter, error) {
	if s == nil || s.reader == nil {
		return domain.ReadFilter{}, ErrAnalyticsUnavailable
	}
	now := s.now()
	to := query.To
	if to.IsZero() {
		to = now
	}
	from := query.From
	if from.IsZero() {
		from = to.Add(-7 * 24 * time.Hour)
	}
	if !from.Before(to) || to.Sub(from) > maxAnalyticsRange || to.After(now.Add(5*time.Minute)) {
		return domain.ReadFilter{}, ErrInvalidAnalyticsQuery
	}
	page, pageSize := query.Page, query.PageSize
	if page == 0 {
		page = 1
	}
	if pageSize == 0 {
		pageSize = 20
	}
	if page < 1 || pageSize < 1 || pageSize > 100 {
		return domain.ReadFilter{}, ErrInvalidAnalyticsQuery
	}
	status := domain.RunStatus(strings.TrimSpace(query.Status))
	if status != "" && !status.IsValid() {
		return domain.ReadFilter{}, ErrInvalidAnalyticsQuery
	}
	return domain.ReadFilter{From: from, To: to, Status: status, Page: page, PageSize: pageSize}, nil
}

func (s *AnalyticsService) listItem(run *domain.Run) RunListItem {
	return RunListItem{
		ID: run.ID, RequestID: run.RequestID, UserID: run.UserID, SessionID: run.SessionID, ActorRole: run.ActorRole,
		Status: string(run.Status), TelemetryStatus: string(run.TelemetryStatus), HTTPStatus: run.HTTPStatus,
		FailureCategory: run.FailureCategory, FailureCode: run.FailureCode, DurationMS: run.DurationMS,
		ModelCallCount: run.ModelCallCount, ToolCallCount: run.ToolCallCount,
		PromptTokens: run.PromptTokens, CompletionTokens: run.CompletionTokens, TotalTokens: run.TotalTokens,
		CachedTokens: run.CachedTokens, EstimatedCost: estimateCost(s.pricing, run.PromptTokens, run.CompletionTokens, run.CachedTokens),
		CreatedAt: run.CreatedAt,
	}
}

func estimateCost(pricing Pricing, prompt, completion, cached *int64) CostEstimate {
	if !pricing.Enabled || !pricing.Valid() || prompt == nil || completion == nil || *prompt < 0 || *completion < 0 {
		return CostEstimate{}
	}
	cachedTokens := int64(0)
	if cached != nil && *cached > 0 {
		cachedTokens = *cached
		if cachedTokens > *prompt {
			cachedTokens = *prompt
		}
	}
	nonCached := *prompt - cachedTokens
	// USD / 1M tokens equals micro-USD / token. Round once after summing the
	// components so tiny requests do not accumulate per-component bias.
	microUSD := float64(nonCached)*pricing.InputUSDPerMillion +
		float64(cachedTokens)*pricing.CachedInputUSDPerMillion +
		float64(*completion)*pricing.OutputUSDPerMillion
	if microUSD < 0 || microUSD > math.MaxInt64 {
		return CostEstimate{}
	}
	return CostEstimate{Known: true, MicroUSD: int64(math.Round(microUSD))}
}

func finiteNonNegative(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func cloneCounts(source map[string]int64) map[string]int64 {
	result := make(map[string]int64, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}
