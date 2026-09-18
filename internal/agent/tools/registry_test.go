package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"SWPUCAT/internal/agent/business"
	"SWPUCAT/internal/agent/confirmation"

	einotool "github.com/cloudwego/eino/components/tool"
)

func TestWriteToolsOnlyCollectSignedConfirmationProposals(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "write endpoint must not be called while proposing", http.StatusInternalServerError)
	}))
	defer server.Close()
	client, err := business.NewClient(server.URL, strings.Repeat("i", 32), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := confirmation.NewSigner(strings.Repeat("s", 32), 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := NewRegistry(client, true, signer).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	collector := business.NewActionProposalCollector()
	ctx := business.WithActionProposalCollector(
		business.WithRequestContext(context.Background(), business.RequestContext{
			Authorization: "Bearer captain-token", UserID: 7, Role: "captain", SessionID: "session-7",
		}),
		collector,
	)
	tests := []struct {
		name      string
		arguments string
	}{
		{name: confirmation.ToolClockIn, arguments: `{}`},
		{name: confirmation.ToolClockOut, arguments: `{}`},
		{name: confirmation.ToolDraftAnnouncement, arguments: `{"title":"值班安排","content":"请按新表值班","pinned":true}`},
	}
	for _, test := range tests {
		invokable, ok := findRegisteredTool(t, registered, test.name).(einotool.InvokableTool)
		if !ok {
			t.Fatalf("%s is not invokable", test.name)
		}
		output, runErr := invokable.InvokableRun(ctx, test.arguments)
		if runErr != nil {
			t.Fatalf("%s invocation error = %v", test.name, runErr)
		}
		var safeResult actionProposalToolResult
		if err := json.Unmarshal([]byte(output), &safeResult); err != nil {
			t.Fatalf("decode %s output: %v", test.name, err)
		}
		if safeResult.Status != "confirmation_required" || safeResult.Tool != test.name || strings.Contains(output, ".") {
			t.Fatalf("unsafe or invalid %s output = %s", test.name, output)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("proposal tools issued %d business HTTP requests", requests.Load())
	}
	proposals := collector.List()
	if len(proposals) != len(tests) {
		t.Fatalf("proposal count = %d, want %d", len(proposals), len(tests))
	}
	for index, proposal := range proposals {
		if proposal.Tool != tests[index].name || proposal.Token == "" {
			t.Fatalf("proposal %d = %+v", index, proposal)
		}
		if _, _, err := signer.Verify(proposal.Token, 7, "session-7", proposal.Tool, proposal.Parameters); err != nil {
			t.Fatalf("verify proposal %d: %v", index, err)
		}
	}
}

func TestPersonalWorkHoursToolIsReadOnlyAndUserScoped(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/checkin/me/work-hours" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer member-token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"as_of_date":"2026-09-17","user_id":7,"nickname":"成员甲","week":{"starts_on":"2026-09-14","ends_on":"2026-09-17","total_minutes":240,"total_hours":4,"record_days":2,"attendance_state":"normal","anomalies":[]},"month":{"starts_on":"2026-09-01","ends_on":"2026-09-17","total_minutes":960,"total_hours":16,"record_days":8,"attendance_state":"normal","anomalies":[]},"weekly_requirement":{"grade":3,"required_minutes":360,"required_hours":6,"is_met":false,"shortfall_minutes":120,"shortfall_hours":2,"status":"below_target"}}}`))
	}))
	defer server.Close()
	client, err := business.NewClient(server.URL, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := NewRegistry(client, false).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := findRegisteredTool(t, registered, "get_my_work_hours")
	info, err := item.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schemaJSON, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(schemaJSON), `"user_id"`) {
		t.Fatalf("personal tool unexpectedly accepts user_id: %s", schemaJSON)
	}
	invokable, ok := item.(einotool.InvokableTool)
	if !ok {
		t.Fatal("get_my_work_hours is not invokable")
	}
	ctx := business.WithRequestContext(context.Background(), business.RequestContext{Authorization: "Bearer member-token", UserID: 7, Role: "member"})
	output, err := invokable.InvokableRun(ctx, `{}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
	var result business.PersonalWorkHours
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if result.UserID != 7 || result.Week.TotalMinutes != 240 || result.Month.TotalMinutes != 960 {
		t.Fatalf("tool output = %+v", result)
	}
}

func TestPersonalCheckinHistoryToolDefaultsAndBoundsLimit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/checkin/records" || r.URL.Query().Get("limit") != "20" {
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer member-token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":[{"id":9,"type":"out","date":"2026-09-17","time":"18:00:00"}]}`))
	}))
	defer server.Close()
	client, err := business.NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := NewRegistry(client, false).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := findRegisteredTool(t, registered, "get_my_checkin_history")
	info, err := item.Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	schemaJSON, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(schemaJSON), `"user_id"`) {
		t.Fatalf("history tool unexpectedly accepts user_id: %s", schemaJSON)
	}
	invokable, ok := item.(einotool.InvokableTool)
	if !ok {
		t.Fatal("get_my_checkin_history is not invokable")
	}
	ctx := business.WithRequestContext(context.Background(), business.RequestContext{Authorization: "Bearer member-token", UserID: 7, Role: "member"})
	for _, arguments := range []string{`{"limit":-1}`, `{"limit":101}`} {
		if _, err := invokable.InvokableRun(ctx, arguments); err == nil {
			t.Fatalf("arguments %s unexpectedly succeeded", arguments)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid input issued %d HTTP requests", requests.Load())
	}
	output, err := invokable.InvokableRun(ctx, `{}`)
	if err != nil {
		t.Fatalf("default invocation error = %v", err)
	}
	var result checkinHistoryResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if requests.Load() != 1 || result.Limit != 20 || len(result.Records) != 1 || result.Records[0].ID != 9 {
		t.Fatalf("history result = %+v, requests=%d", result, requests.Load())
	}
}

func TestCheckinRequirementsToolIsAvailableToMembers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/checkin/requirements" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer member-token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"requirements":[{"grade":1,"minutes":600},{"grade":4,"minutes":240}]}}`))
	}))
	defer server.Close()
	client, err := business.NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := NewRegistry(client, false).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := findRegisteredTool(t, registered, "get_checkin_requirements")
	invokable, ok := item.(einotool.InvokableTool)
	if !ok {
		t.Fatal("get_checkin_requirements is not invokable")
	}
	ctx := business.WithRequestContext(context.Background(), business.RequestContext{Authorization: "Bearer member-token", UserID: 7, Role: "member"})
	output, err := invokable.InvokableRun(ctx, `{}`)
	if err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
	var result business.CheckinRequirements
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode tool output: %v", err)
	}
	if len(result.Requirements) != 2 || result.Requirements[0].Grade != 1 || result.Requirements[1].Minutes != 240 {
		t.Fatalf("requirements result = %+v", result)
	}
}

func TestRecommendKnowledgeCollectsHybridSourceMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/knowledge/recommendations" || r.URL.Query().Get("q") != "请求为什么卡住" {
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":[{"item_id":42,"type":"file","name":"稳定性手册.pdf","description":"后端工程手册","download_url":"/api/knowledge/download/42","category_name":"工程实践","source_kind":"content","excerpt":"使用 context 取消超时请求。","score":0.82,"match_mode":"hybrid","reason":"关键词与语义综合匹配"}]}`))
	}))
	defer server.Close()
	client, err := business.NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := NewRegistry(client, false).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	invokable, ok := findRegisteredTool(t, registered, "recommend_knowledge").(einotool.InvokableTool)
	if !ok {
		t.Fatal("recommend_knowledge is not invokable")
	}
	collector := business.NewKnowledgeSourceCollector()
	ctx := business.WithKnowledgeSourceCollector(
		business.WithRequestContext(context.Background(), business.RequestContext{Authorization: "Bearer member-token", UserID: 7, Role: "member"}),
		collector,
	)
	if _, err := invokable.InvokableRun(ctx, `{"need":"请求为什么卡住"}`); err != nil {
		t.Fatalf("InvokableRun() error = %v", err)
	}
	sources := collector.List()
	if len(sources) != 1 || sources[0].ID != 42 || sources[0].MatchMode != "hybrid" || sources[0].Excerpt == "" {
		t.Fatalf("recommendation source metadata was not collected: %+v", sources)
	}
	if sources[0].DownloadURL != "/api/knowledge/download/42" || sources[0].Score != 0.82 {
		t.Fatalf("recommendation source link or score was lost: %+v", sources[0])
	}
}

func TestReportPeriodsToolValidatesFiltersSortsAndLimits(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/api/report-periods" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer member-token" {
			t.Fatalf("authorization header = %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":[{"id":9,"period_key":"2026-06","starts_on":"2026-06-01","ends_on":"2026-06-30","timezone":"Asia/Shanghai","status":"open","roster_version":1,"member_count":10},{"id":11,"period_key":"2026-08","starts_on":"2026-08-01","ends_on":"2026-08-31","timezone":"Asia/Shanghai","status":"open","roster_version":1,"member_count":12},{"id":13,"period_key":"2026-10","starts_on":"2026-10-01","ends_on":"2026-10-31","timezone":"Asia/Shanghai","status":"closed","roster_version":1,"member_count":12},{"id":12,"period_key":"2026-09","starts_on":"2026-09-01","ends_on":"2026-09-30","timezone":"Asia/Shanghai","status":"open","roster_version":2,"member_count":15},{"id":8,"period_key":"2026-07","starts_on":"2026-07-01","ends_on":"2026-07-31","timezone":"Asia/Shanghai","status":"open","roster_version":1,"member_count":9}]}`))
	}))
	defer server.Close()
	client, err := business.NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := NewRegistry(client, false).Build(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	item := findRegisteredTool(t, registered, "list_report_periods")
	invokable, ok := item.(einotool.InvokableTool)
	if !ok {
		t.Fatal("list_report_periods is not invokable")
	}
	ctx := business.WithRequestContext(context.Background(), business.RequestContext{Authorization: "Bearer member-token", UserID: 7, Role: "member"})
	for _, arguments := range []string{
		`{"period_key":"2026-13"}`,
		`{"period_key":"26-09"}`,
		`{"status":"draft"}`,
		`{"limit":-1}`,
		`{"limit":25}`,
	} {
		if _, err := invokable.InvokableRun(ctx, arguments); err == nil {
			t.Fatalf("arguments %s unexpectedly succeeded", arguments)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid filters issued %d HTTP requests", requests.Load())
	}

	output, err := invokable.InvokableRun(ctx, `{"status":"open","limit":2}`)
	if err != nil {
		t.Fatalf("filtered invocation error = %v", err)
	}
	var result reportPeriodsResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode filtered output: %v", err)
	}
	if result.TotalPeriods != 4 || !result.Truncated || len(result.Periods) != 2 || result.Periods[0].ID != 12 || result.Periods[1].ID != 11 {
		t.Fatalf("filtered periods = %+v", result)
	}

	result = reportPeriodsResult{}
	output, err = invokable.InvokableRun(ctx, `{"period_key":"2026-08","limit":1}`)
	if err != nil {
		t.Fatalf("exact invocation error = %v", err)
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode exact output: %v", err)
	}
	if result.TotalPeriods != 1 || result.Truncated || len(result.Periods) != 1 || result.Periods[0].ID != 11 {
		t.Fatalf("exact periods = %+v", result)
	}

	result = reportPeriodsResult{}
	output, err = invokable.InvokableRun(ctx, `{"period_key":"2025-01"}`)
	if err != nil {
		t.Fatalf("empty invocation error = %v", err)
	}
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("decode empty output: %v", err)
	}
	if result.TotalPeriods != 0 || result.Periods == nil || len(result.Periods) != 0 {
		t.Fatalf("empty periods = %+v", result)
	}
	if requests.Load() != 3 {
		t.Fatalf("valid filters issued %d HTTP requests, want 3", requests.Load())
	}
}

func findRegisteredTool(t *testing.T, items []einotool.BaseTool, name string) einotool.BaseTool {
	t.Helper()
	for _, item := range items {
		info, err := item.Info(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if info.Name == name {
			return item
		}
	}
	t.Fatalf("tool %q is not registered", name)
	return nil
}

func TestMergeReportsFiltersMineAndDeduplicatesReviewedVersions(t *testing.T) {
	submitted := []business.Report{{ID: 1, AuthorID: 7}, {ID: 2, AuthorID: 8}}
	reviewed := []business.Report{{ID: 1, AuthorID: 7}, {ID: 3, AuthorID: 7}}
	items := mergeReports(submitted, reviewed, "mine", 7)
	if len(items) != 2 || items[0].ID != 1 || items[1].ID != 3 {
		t.Fatalf("unexpected merged reports: %+v", items)
	}
}

func TestTruncateReportBodyMarksLongMaterial(t *testing.T) {
	body, truncated := truncateReportBody(string(make([]rune, maxReportBodyRunes+1)))
	if !truncated || len([]rune(body)) != maxReportBodyRunes+1 {
		t.Fatalf("unexpected truncation: truncated=%t runes=%d", truncated, len([]rune(body)))
	}
}

func TestReportMaterialsPageLimitIsBounded(t *testing.T) {
	if maxReportMaterialsPerPage != 8 {
		t.Fatalf("page limit = %d, want 8", maxReportMaterialsPerPage)
	}
}

func TestBoundReportMaterialsKeepsPageWithinOutputBudget(t *testing.T) {
	materials := make([]reportMaterial, maxReportMaterialsPerPage)
	for i := range materials {
		materials[i] = reportMaterial{
			ReportID:     int64(i + 1),
			AuthorName:   strings.Repeat("成员", maxReportMetadataRunes+32),
			Title:        strings.Repeat("标题", maxReportMetadataRunes+32),
			BodyMarkdown: strings.Repeat("报告正文 ", maxReportBodyRunes*3),
			ParsedJSON:   map[string]any{"details": strings.Repeat("结构化内容 ", maxReportParsedJSONRunes)},
		}
	}

	result := boundReportMaterials(reportMaterialsResult{PeriodID: 1, Scope: "team", Limit: maxReportMaterialsPerPage, Materials: materials})
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal bounded materials: %v", err)
	}
	if got := len([]rune(string(data))); got > maxReportMaterialsOutputRunes {
		t.Fatalf("bounded output has %d runes, want <= %d", got, maxReportMaterialsOutputRunes)
	}
	for i, item := range result.Materials {
		if !item.Truncated {
			t.Errorf("material %d is not marked truncated", i)
		}
	}
}

func TestBoundReportMaterialsAllocatesBodyBudgetAcrossReports(t *testing.T) {
	materials := []reportMaterial{
		{ReportID: 1, BodyMarkdown: strings.Repeat("a", maxReportBodyRunes*2)},
		{ReportID: 2, BodyMarkdown: strings.Repeat("b", maxReportBodyRunes*2)},
		{ReportID: 3, BodyMarkdown: strings.Repeat("c", maxReportBodyRunes*2)},
	}
	result := boundReportMaterials(reportMaterialsResult{Materials: materials})
	total := 0
	for _, item := range result.Materials {
		total += len([]rune(item.BodyMarkdown))
	}
	if total > maxReportMaterialsBodyRunes+len(result.Materials) {
		t.Fatalf("body budget = %d, want <= %d including truncation markers", total, maxReportMaterialsBodyRunes+len(result.Materials))
	}
	for _, item := range result.Materials {
		if !item.Truncated {
			t.Fatal("expected every oversized report to carry a truncation marker")
		}
	}
}

func TestBoundReportMaterialsStripsOversizedEscapedMetadata(t *testing.T) {
	metadata := strings.Repeat("\x00", maxReportMetadataRunes+64)
	materials := make([]reportMaterial, maxReportMaterialsPerPage)
	for i := range materials {
		materials[i] = reportMaterial{
			ReportID:    int64(i + 1),
			AuthorName:  metadata,
			Status:      metadata,
			Title:       metadata,
			ParseStatus: metadata,
			ParseError:  metadata,
		}
	}
	result := boundReportMaterials(reportMaterialsResult{Materials: materials})
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal bounded metadata: %v", err)
	}
	if got := len([]rune(string(data))); got > maxReportMaterialsOutputRunes {
		t.Fatalf("bounded metadata output has %d runes, want <= %d", got, maxReportMaterialsOutputRunes)
	}
	if result.Materials[0].AuthorName != "" || !result.Materials[0].Truncated {
		t.Fatalf("expected optional metadata to be stripped: %+v", result.Materials[0])
	}
}

func TestMarshalKnowledgeSearchResultKeepsRequiredFieldsWithinBudget(t *testing.T) {
	sources := make([]business.KnowledgeSource, maxKnowledgeToolSources)
	for i := range sources {
		sources[i] = business.KnowledgeSource{
			ID:           int64(i + 1),
			Type:         "file",
			Name:         "资料-" + string(rune('A'+i)),
			Description:  strings.Repeat("说明内容 ", 5000),
			URL:          "https://example.com/knowledge/" + string(rune('a'+i)),
			DownloadURL:  "/api/knowledge/download/" + string(rune('0'+i)),
			CategoryName: "制度",
			Excerpt:      strings.Repeat("正文摘要 ", 1000),
		}
	}
	encoded, err := marshalKnowledgeSearchResult(strings.Repeat("问题", 1000), sources)
	if err != nil {
		t.Fatalf("marshal knowledge result: %v", err)
	}
	if got := len([]rune(encoded)); got > maxKnowledgeToolOutputRunes {
		t.Fatalf("knowledge output has %d runes, want <= %d", got, maxKnowledgeToolOutputRunes)
	}
	var result knowledgeSearchResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode bounded knowledge result: %v", err)
	}
	if len(result.Sources) == 0 || !result.Truncated {
		t.Fatalf("expected bounded result with sources and truncation marker: %+v", result)
	}
	for i, source := range result.Sources {
		if source.ID == 0 || source.Name == "" {
			t.Errorf("source %d lost required identity: %+v", i, source)
		}
		if source.URL == "" && source.DownloadURL == "" {
			t.Errorf("source %d lost both links: %+v", i, source)
		}
	}
}

func TestMarshalKnowledgeSearchResultDropsSourcesOnlyAfterTextBudget(t *testing.T) {
	longURL := "https://example.com/" + strings.Repeat("x", maxKnowledgeToolURLRunes*4)
	sources := []business.KnowledgeSource{
		{ID: 1, Name: "第一份", Description: strings.Repeat("摘要", 2000), URL: longURL},
		{ID: 2, Name: "第二份", Description: strings.Repeat("摘要", 2000), URL: longURL},
	}
	encoded, err := marshalKnowledgeSearchResult("查询", sources)
	if err != nil {
		t.Fatalf("marshal knowledge result: %v", err)
	}
	if got := len([]rune(encoded)); got > maxKnowledgeToolOutputRunes {
		t.Fatalf("knowledge output has %d runes, want <= %d", got, maxKnowledgeToolOutputRunes)
	}
	var result knowledgeSearchResult
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		t.Fatalf("decode bounded knowledge result: %v", err)
	}
	if len(result.Sources) != len(sources) {
		t.Fatalf("normal bounded sources were dropped: got %d want %d", len(result.Sources), len(sources))
	}
	for _, source := range result.Sources {
		if source.ID == 0 || source.Name == "" {
			t.Fatalf("source identity missing: %+v", source)
		}
	}
}
