package telemetry

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"
)

func TestNormalizeAndSummarizeUsage(t *testing.T) {
	if got := normalizeUsage(nil); got != nil {
		t.Fatalf("normalizeUsage(nil) = %#v, want nil", got)
	}
	if got := normalizeUsage(&schema.TokenUsage{}); got != nil {
		t.Fatalf("normalizeUsage(zero) = %#v, want nil", got)
	}
	if got := normalizeUsage(&schema.TokenUsage{PromptTokens: -1, TotalTokens: -1}); got != nil {
		t.Fatalf("normalizeUsage(negative) = %#v, want nil", got)
	}
	maximumPerCall := int(^uint(0)>>1) / maxRecordedModelCalls
	if got := normalizeUsage(&schema.TokenUsage{PromptTokens: maximumPerCall + 1}); got != nil {
		t.Fatalf("normalizeUsage(oversize) = %#v, want nil", got)
	}

	first := &TokenUsage{PromptTokens: 10, CompletionTokens: 4, TotalTokens: 14, CachedTokens: 3, ReasoningTokens: 2}
	second := &TokenUsage{PromptTokens: 7, CompletionTokens: 5, TotalTokens: 12, CachedTokens: 1, ReasoningTokens: 4}
	summary := summarizeUsage([]ModelCall{{Usage: first}, {Usage: nil}, {Usage: second}}, 1)
	if summary.ReportedModelCalls != 2 || summary.MissingModelCalls != 2 {
		t.Fatalf("usage call counts = reported %d missing %d, want 2/2", summary.ReportedModelCalls, summary.MissingModelCalls)
	}
	wantTotals := TokenUsage{PromptTokens: 17, CompletionTokens: 9, TotalTokens: 26, CachedTokens: 4, ReasoningTokens: 6}
	if summary.ReportedTotals != wantTotals {
		t.Fatalf("reported totals = %#v, want %#v", summary.ReportedTotals, wantTotals)
	}
	if summary.CompleteTotals != nil {
		t.Fatalf("complete totals = %#v, want nil when usage is missing", summary.CompleteTotals)
	}

	complete := summarizeUsage([]ModelCall{{Usage: first}, {Usage: second}}, 0)
	if complete.CompleteTotals == nil || *complete.CompleteTotals != wantTotals {
		t.Fatalf("complete totals = %#v, want %#v", complete.CompleteTotals, wantTotals)
	}
	maximum := int(^uint(0) >> 1)
	saturated := summarizeUsage([]ModelCall{{Usage: &TokenUsage{TotalTokens: maximum}}, {Usage: &TokenUsage{TotalTokens: 1}}}, 0)
	if saturated.ReportedTotals.TotalTokens != maximum {
		t.Fatalf("overflowing total = %d, want saturation at %d", saturated.ReportedTotals.TotalTokens, maximum)
	}
}

func TestCollectorSnapshotAndDeepCopy(t *testing.T) {
	base := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	clockCall := 0
	collector := newCollector(func() time.Time {
		value := base.Add(time.Duration(clockCall) * time.Second)
		clockCall++
		return value
	})

	model := collector.beginModel("provider", "model", 3, 4)
	collector.finishModel(model, modelOutcome{
		status:       "error",
		finishReason: "length",
		usage:        &TokenUsage{PromptTokens: 8, CompletionTokens: 2, TotalTokens: 10},
		toolRequests: []ToolRequest{{CallID: "call-1", Name: "search_knowledge"}},
		err:          &ErrorInfo{Category: "provider", Code: "bad_request"},
	})
	tool := collector.beginTool("search_knowledge", "call-1", `{"query":"private query"}`)
	collector.finishTool(tool, `{"sources":[]}`, &ErrorInfo{Category: "execution", Code: "failed"}, "error")

	first := collector.Snapshot()
	if first.ModelCallCount != 1 || first.ToolCallCount != 1 || len(first.ModelCalls) != 1 || len(first.ToolCalls) != 1 {
		t.Fatalf("unexpected snapshot counts: %#v", first)
	}
	if first.ModelCalls[0].Sequence != 1 || first.ToolCalls[0].Sequence != 2 {
		t.Fatalf("sequences = model %d tool %d, want 1/2", first.ModelCalls[0].Sequence, first.ToolCalls[0].Sequence)
	}
	if first.ModelCalls[0].DurationMS != 1000 || first.ToolCalls[0].DurationMS != 1000 || first.DurationMS != 5000 {
		t.Fatalf("durations = model %d tool %d run %d", first.ModelCalls[0].DurationMS, first.ToolCalls[0].DurationMS, first.DurationMS)
	}

	first.ModelCalls[0].Usage.PromptTokens = 999
	first.ModelCalls[0].Error.Code = "mutated"
	first.ModelCalls[0].ToolRequests[0].Name = "mutated"
	first.ToolCalls[0].Error.Code = "mutated"
	second := collector.Snapshot()
	if second.ModelCalls[0].Usage.PromptTokens != 8 || second.ModelCalls[0].Error.Code != "bad_request" || second.ModelCalls[0].ToolRequests[0].Name != "search_knowledge" {
		t.Fatalf("model snapshot is not a deep copy: %#v", second.ModelCalls[0])
	}
	if second.ToolCalls[0].Error.Code != "failed" {
		t.Fatalf("tool snapshot is not a deep copy: %#v", second.ToolCalls[0])
	}
	if data, err := json.Marshal(second); err != nil || !json.Valid(data) {
		t.Fatalf("snapshot JSON invalid: data=%q err=%v", data, err)
	}
}

func TestCollectorConcurrentReservationLimits(t *testing.T) {
	collector := NewCollector()
	runConcurrentSpans(t, 100, func(index int) func() {
		span := collector.beginModel("provider", "model", index, 1)
		return func() { collector.finishModel(span, modelOutcome{status: "success"}) }
	})
	runConcurrentSpans(t, 200, func(index int) func() {
		span := collector.beginTool("clock_in", "call", `{}`)
		return func() { collector.finishTool(span, `{"status":"ok"}`, nil, "success") }
	})

	snapshot := collector.Snapshot()
	if snapshot.ModelCallCount != 100 || len(snapshot.ModelCalls) != maxRecordedModelCalls || snapshot.DroppedModelCalls != 36 {
		t.Fatalf("model limits = count %d recorded %d dropped %d", snapshot.ModelCallCount, len(snapshot.ModelCalls), snapshot.DroppedModelCalls)
	}
	if snapshot.ToolCallCount != 200 || len(snapshot.ToolCalls) != maxRecordedToolCalls || snapshot.DroppedToolCalls != 72 {
		t.Fatalf("tool limits = count %d recorded %d dropped %d", snapshot.ToolCallCount, len(snapshot.ToolCalls), snapshot.DroppedToolCalls)
	}
	seen := make(map[uint64]struct{}, len(snapshot.ModelCalls)+len(snapshot.ToolCalls))
	for _, call := range snapshot.ModelCalls {
		if _, exists := seen[call.Sequence]; exists {
			t.Fatalf("duplicate model sequence %d", call.Sequence)
		}
		seen[call.Sequence] = struct{}{}
	}
	for _, call := range snapshot.ToolCalls {
		if _, exists := seen[call.Sequence]; exists {
			t.Fatalf("duplicate tool sequence %d", call.Sequence)
		}
		seen[call.Sequence] = struct{}{}
	}
}

func TestCollectorInFlightCountsAndSafeConfigurationNames(t *testing.T) {
	collector := NewCollector()
	model := collector.beginModel("Bearer SECRET_PROVIDER_913", "model-ghp_SECRET_MODEL_913", 1, 0)
	tool := collector.beginTool("clock_in", "call-id", `{}`)

	inFlight := collector.Snapshot()
	if inFlight.ModelCallCount != 1 || inFlight.ToolCallCount != 1 || len(inFlight.ModelCalls) != 0 || len(inFlight.ToolCalls) != 0 || inFlight.Usage.MissingModelCalls != 1 {
		t.Fatalf("in-flight snapshot = %#v", inFlight)
	}

	collector.finishModel(model, modelOutcome{status: "success"})
	collector.finishTool(tool, `{"type":"in","date":"2026-09-18"}`, nil, "success")
	finished := collector.Snapshot()
	if finished.ModelCalls[0].Provider != "[invalid]" || finished.ModelCalls[0].Model != "[invalid]" {
		t.Fatalf("unsafe configuration names were retained: %#v", finished.ModelCalls[0])
	}
	data, _ := json.Marshal(finished)
	for _, secret := range []string{"SECRET_PROVIDER_913", "SECRET_MODEL_913"} {
		if strings.Contains(string(data), secret) {
			t.Fatalf("configuration secret leaked %q: %s", secret, data)
		}
	}
}

func runConcurrentSpans(t *testing.T, count int, begin func(int) func()) {
	t.Helper()
	var ready sync.WaitGroup
	var done sync.WaitGroup
	ready.Add(count)
	done.Add(count)
	release := make(chan struct{})
	for i := 0; i < count; i++ {
		go func(index int) {
			defer done.Done()
			finish := begin(index)
			ready.Done()
			<-release
			finish()
		}(i)
	}
	ready.Wait()
	close(release)
	done.Wait()
}
