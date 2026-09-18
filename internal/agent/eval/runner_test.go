package agenteval

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"SWPUCAT/internal/agent"
	"SWPUCAT/internal/agent/business"
	"SWPUCAT/internal/agent/telemetry"
	"SWPUCAT/internal/infrastructure/config"
)

func TestValidateReportsCompleteOfflineSuite(t *testing.T) {
	report, err := Validate(DefaultSuite)
	if err != nil {
		t.Fatalf("Validate(%q) error = %v", DefaultSuite, err)
	}
	if report.Mode != "validate" || report.Suite != DefaultSuite {
		t.Fatalf("report mode/suite = %q/%q, want validate/%s", report.Mode, report.Suite, DefaultSuite)
	}
	if report.SuiteCases != 54 || report.SelectedCases != 54 || report.Coverage.Cases != 54 {
		t.Fatalf("case counts = suite:%d selected:%d coverage:%d, want 54/54/54", report.SuiteCases, report.SelectedCases, report.Coverage.Cases)
	}
	if report.Repeat != 1 || !report.Success || len(report.Runs) != 0 {
		t.Fatalf("offline report execution fields = repeat:%d success:%v runs:%d", report.Repeat, report.Success, len(report.Runs))
	}
	if !strings.Contains(report.ValidationNotice, "不代表模型质量通过") {
		t.Fatalf("ValidationNotice = %q, want explicit offline-only notice", report.ValidationNotice)
	}

	for _, role := range []string{"member", "captain", "super_admin"} {
		role := role
		t.Run(role, func(t *testing.T) {
			filtered, err := ValidateRole(DefaultSuite, role)
			if err != nil {
				t.Fatalf("ValidateRole(%q, %q) error = %v", DefaultSuite, role, err)
			}
			if filtered.Role != role || filtered.SuiteCases != 54 || filtered.SelectedCases != 18 || filtered.Coverage.Cases != 18 {
				t.Fatalf("filtered report = role:%q suite:%d selected:%d coverage:%d", filtered.Role, filtered.SuiteCases, filtered.SelectedCases, filtered.Coverage.Cases)
			}
			if got := filtered.Coverage.Roles[role]; got != 18 {
				t.Fatalf("Coverage.Roles[%q] = %d, want 18", role, got)
			}
			if len(filtered.Coverage.Roles) != 1 {
				t.Fatalf("Coverage.Roles = %#v, want only selected role", filtered.Coverage.Roles)
			}
			if filtered.ValidationNotice == "" || len(filtered.Runs) != 0 {
				t.Fatalf("role validation is not clearly offline: notice=%q runs=%d", filtered.ValidationNotice, len(filtered.Runs))
			}
		})
	}
}

func TestValidateRoleRejectsInvalidRole(t *testing.T) {
	report, err := ValidateRole(DefaultSuite, "owner")
	if err == nil {
		t.Fatal("ValidateRole with invalid role returned nil error")
	}
	if report != nil {
		t.Fatalf("ValidateRole report = %#v, want nil on invalid role", report)
	}
	if !strings.Contains(err.Error(), "role must be one of") {
		t.Fatalf("ValidateRole error = %q, want supported-role guidance", err)
	}
}

func TestValidateCoverageRequiresEveryProductionTool(t *testing.T) {
	report, err := Validate(DefaultSuite)
	if err != nil {
		t.Fatalf("Validate(%q) error = %v", DefaultSuite, err)
	}
	tools := ToolNames()
	if len(tools) != 13 {
		t.Fatalf("ToolNames count = %d, want 13", len(tools))
	}
	if len(report.Coverage.Tools) != len(tools) {
		t.Fatalf("Coverage.Tools = %#v, want exactly the %d production tools", report.Coverage.Tools, len(tools))
	}
	for _, toolName := range tools {
		if report.Coverage.Tools[toolName] < 1 {
			t.Errorf("required-call coverage for %q = %d, want at least 1", toolName, report.Coverage.Tools[toolName])
		}
	}

	coverage := coverageFor([]Case{{
		Role:          "member",
		AllowedTools:  []string{"get_checkin_status", "search_knowledge"},
		RequiredTools: []string{"get_checkin_status"},
	}})
	if coverage.Tools["get_checkin_status"] != 1 {
		t.Fatalf("required tool count = %d, want 1", coverage.Tools["get_checkin_status"])
	}
	if _, ok := coverage.Tools["search_knowledge"]; ok {
		t.Fatalf("allowed-only tool was counted as required coverage: %#v", coverage.Tools)
	}
}

func TestChatRequestConvertsEvaluationCase(t *testing.T) {
	item := Case{
		ID:             "C07",
		Role:           "captain",
		Prompt:         "总结资料",
		RollingSummary: "此前讨论了月报。",
		History: []HistoryMessage{
			{Role: "user", Content: "上一问"},
			{Role: "assistant", Content: "上一答"},
		},
		KnowledgeContext: []KnowledgeContextItem{{
			ID:           301,
			Type:         "article",
			Name:         "打卡规范",
			Description:  "说明",
			URL:          "https://example.test/knowledge/301",
			DownloadURL:  "https://example.test/knowledge/301/download",
			CategoryName: "制度",
			SourceKind:   "content",
			Excerpt:      "摘要",
			Score:        0.75,
			MatchMode:    "keyword",
			CreatedAt:    "2026-09-18T00:00:00Z",
		}},
	}

	got := chatRequest(item, 3)
	want := agent.ChatRequest{
		Message:         "总结资料",
		SessionID:       "eval-c07-3",
		UserID:          9,
		Role:            "captain",
		HistoryProvided: true,
		History: []agent.HistoryMessage{
			{Role: "user", Content: "上一问"},
			{Role: "assistant", Content: "上一答"},
		},
		RollingSummary: "此前讨论了月报。",
		KnowledgeContext: []business.KnowledgeSource{{
			ID:           301,
			Type:         "article",
			Name:         "打卡规范",
			Description:  "说明",
			URL:          "https://example.test/knowledge/301",
			DownloadURL:  "https://example.test/knowledge/301/download",
			CategoryName: "制度",
			SourceKind:   "content",
			Excerpt:      "摘要",
			Score:        0.75,
			MatchMode:    "keyword",
			CreatedAt:    "2026-09-18T00:00:00Z",
		}},
		RequestID:     "eval-c07-3",
		Authorization: "Bearer eval/C07",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chatRequest() = %#v, want %#v", got, want)
	}
}

func TestObservationFromConvertsTelemetryAndLedger(t *testing.T) {
	response := agent.ChatResponse{
		Answer:  "完成 [知识#301]",
		Sources: []business.KnowledgeSource{{ID: 301}, {ID: 304}},
		Telemetry: telemetry.Snapshot{ToolCalls: []telemetry.ToolCall{
			{
				Name:      "get_my_checkin_history",
				Status:    "success",
				Arguments: telemetry.PayloadCapture{JSON: `{"limit":5,"include_unpaired":true}`},
			},
			{
				Name:      "search_knowledge",
				Status:    "error",
				Arguments: telemetry.PayloadCapture{JSON: `{"query":`},
			},
		}},
	}
	ledger := CaseLedger{
		Requests: []LedgerRequest{
			{Path: "/api/reports/101/versions/3", Succeeded: true},
			{Path: "/api/reports/102/versions/2", Succeeded: false},
		},
		WriteAttempts: 2, WriteSucceeded: 1,
	}

	got := observationFrom(response, ledger, nil)
	if got.Answer != response.Answer || !reflect.DeepEqual(got.SourceIDs, []int64{301, 304}) {
		t.Fatalf("answer/sources = %q/%v", got.Answer, got.SourceIDs)
	}
	if got.WriteAttempts != 2 || got.WriteSucceeded != 1 {
		t.Fatalf("write counts = attempts:%d succeeded:%d, want 2/1", got.WriteAttempts, got.WriteSucceeded)
	}
	if !reflect.DeepEqual(got.FetchedReportRefs, []string{"[报告#101-v3]"}) {
		t.Fatalf("FetchedReportRefs = %v, want successful version fetch only", got.FetchedReportRefs)
	}
	if len(got.ToolCalls) != 2 {
		t.Fatalf("tool call count = %d, want 2", len(got.ToolCalls))
	}
	if got.ToolCalls[0].Name != "get_my_checkin_history" || got.ToolCalls[0].Status != "success" {
		t.Fatalf("first tool call metadata = %#v", got.ToolCalls[0])
	}
	if got.ToolCalls[0].Arguments["limit"] != json.Number("5") || got.ToolCalls[0].Arguments["include_unpaired"] != true {
		t.Fatalf("decoded arguments = %#v", got.ToolCalls[0].Arguments)
	}
	if len(got.ToolCalls[1].Arguments) != 0 {
		t.Fatalf("malformed arguments = %#v, want empty map", got.ToolCalls[1].Arguments)
	}
	if !strings.Contains(got.ExecutionError, "decode search_knowledge tool arguments") {
		t.Fatalf("ExecutionError = %q, want telemetry decode error", got.ExecutionError)
	}

	executionErr := errors.New("model unavailable")
	got = observationFrom(agent.ChatResponse{}, CaseLedger{}, executionErr)
	if got.ExecutionError != executionErr.Error() {
		t.Fatalf("ExecutionError = %q, want %q", got.ExecutionError, executionErr)
	}
}

func TestLiveConfigIsolatesProductionSettingsAndKeepsLLM(t *testing.T) {
	base := config.AgentConfig{
		Enabled:           false,
		WriteEnabled:      false,
		Port:              6553,
		BaseURL:           "https://production-agent.example.test",
		ServerURL:         "https://production-api.example.test",
		InternalSecret:    "production-internal-secret-should-not-survive",
		LLMProvider:       "openai_compatible",
		LLMModel:          "evaluation-model",
		LLMAPIKey:         "evaluation-api-key",
		LLMBaseURL:        "https://model.example.test/v1",
		LLMTimeout:        37 * time.Second,
		MaxIterations:     31,
		MaxToolCalls:      63,
		WorkerConcurrency: 16,
	}
	fixtureURL := "http://127.0.0.1:43123"

	got := liveConfig(base, fixtureURL, true)
	if !got.Enabled || !got.WriteEnabled {
		t.Fatalf("live enabled flags = enabled:%v write:%v, want true/true", got.Enabled, got.WriteEnabled)
	}
	if got.ServerURL != fixtureURL || got.InternalSecret != FixtureInternalSecret {
		t.Fatalf("business settings = server:%q secret:%q", got.ServerURL, got.InternalSecret)
	}
	if got.ServerURL == base.ServerURL || got.InternalSecret == base.InternalSecret {
		t.Fatal("live config retained production business settings")
	}
	if got.Port != 8090 || got.BaseURL != "http://127.0.0.1:8090" {
		t.Fatalf("service settings = port:%d baseURL:%q", got.Port, got.BaseURL)
	}
	if got.MaxIterations != 4 || got.MaxToolCalls != 8 || got.WorkerConcurrency != 1 {
		t.Fatalf("evaluation budgets = iterations:%d tools:%d concurrency:%d, want 4/8/1", got.MaxIterations, got.MaxToolCalls, got.WorkerConcurrency)
	}
	if got.LLMProvider != base.LLMProvider || got.LLMModel != base.LLMModel || got.LLMAPIKey != base.LLMAPIKey || got.LLMBaseURL != base.LLMBaseURL || got.LLMTimeout != base.LLMTimeout {
		t.Fatalf("LLM settings were not preserved: got %#v base %#v", got, base)
	}
}

func TestRunLoadedLiveExercisesModelToolFixtureAndScorer(t *testing.T) {
	var calls atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{
				"id":"eval-response-1","object":"chat.completion","created":1,"model":"test-model",
				"choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call-status","type":"function","function":{"name":"get_checkin_status","arguments":"{}"}}]},"finish_reason":"tool_calls"}],
				"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}
			}`)
			return
		}
		_, _ = io.WriteString(w, `{
			"id":"eval-response-2","object":"chat.completion","created":1,"model":"test-model",
			"choices":[{"index":0,"message":{"role":"assistant","content":"你今天尚未打卡。"},"finish_reason":"stop"}],
			"usage":{"prompt_tokens":12,"completion_tokens":4,"total_tokens":16}
		}`)
	}))
	defer modelServer.Close()

	suite := &Suite{
		Manifest: Manifest{SuiteID: "integration"},
		Cases: []Case{{
			ID: "M01", Role: "member", Prompt: "我今天打卡了吗？",
			AllowedTools: []string{"get_checkin_status"}, RequiredTools: []string{"get_checkin_status"},
			ToolArguments:        map[string][]map[string]any{"get_checkin_status": {{}}},
			AnswerMustContainAny: []string{"尚未打卡", "idle"}, MaxToolCalls: 1,
		}},
		Fixtures: BusinessFixtures{Version: 1, Tools: map[string]map[string]json.RawMessage{
			"get_checkin_status": {"default": json.RawMessage(`{"status":"idle"}`)},
		}},
	}
	report, err := runLoadedLive(context.Background(), "integration", "member", 1, config.AgentConfig{
		LLMProvider: "openai_compatible", LLMModel: "test-model", LLMAPIKey: "test-key",
		LLMBaseURL: modelServer.URL + "/v1", LLMTimeout: 3 * time.Second,
	}, suite)
	if err != nil {
		t.Fatalf("runLoadedLive() error = %v", err)
	}
	if !report.Success || report.PassedRuns != 1 || report.FailedRuns != 0 || len(report.Runs) != 1 {
		t.Fatalf("live integration report = %#v", report)
	}
	if calls.Load() != 2 {
		t.Fatalf("model calls = %d, want tool selection plus final answer", calls.Load())
	}
	if got := report.Runs[0].Observation.ToolCalls; len(got) != 1 || got[0].Name != "get_checkin_status" || got[0].Status != "success" {
		t.Fatalf("tool observation = %#v", got)
	}
}
