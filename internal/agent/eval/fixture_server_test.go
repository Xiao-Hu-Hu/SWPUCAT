package agenteval

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"SWPUCAT/internal/agent/business"
)

func TestFixtureServerSelectsCaseVariantAndDefaults(t *testing.T) {
	suite := fixtureServerTestSuite(
		[]Case{{
			ID: "M01", Role: "member",
			AllowedTools: []string{"get_checkin_status", "get_checkin_requirements"},
			Fixture:      map[string]string{"get_checkin_status": "working"},
		}},
		map[string]map[string]json.RawMessage{
			"get_checkin_status": {
				"default": json.RawMessage(`{"status":"not_clocked_in"}`),
				"working": json.RawMessage(`{"status":"working","clock_in":"09:12:00"}`),
			},
			"get_checkin_requirements": {
				"default": json.RawMessage(`{"requirements":[{"grade":3,"minutes":360}]}`),
			},
		},
	)
	server := startFixtureServer(t, suite)
	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	client := fixtureBusinessClient(t, server)
	ctx := fixtureCaseContext("M01", "member")

	status, err := client.CheckinStatus(ctx)
	if err != nil || status.Status != "working" || status.ClockIn != "09:12:00" {
		t.Fatalf("CheckinStatus() = %+v, err=%v", status, err)
	}
	requirements, err := client.GetCheckinRequirements(ctx)
	if err != nil || len(requirements.Requirements) != 1 || requirements.Requirements[0].Minutes != 360 {
		t.Fatalf("GetCheckinRequirements() = %+v, err=%v", requirements, err)
	}

	ledger := server.Ledger("M01")
	if len(ledger.Requests) != 2 || ledger.Requests[0].FixtureVariant != "working" || ledger.Requests[1].FixtureVariant != "default" {
		t.Fatalf("unexpected ledger: %+v", ledger)
	}
}

func TestFixtureServerServesReportListsAndVersionsFromOneVariant(t *testing.T) {
	fixture := json.RawMessage(`{
		"period_id": 31,
		"reports": {
			"submitted": [{"id":101,"period_id":31,"status":"submitted","current_version":3}],
			"reviewed": [{"id":102,"period_id":31,"status":"reviewed","current_version":2}]
		},
		"versions": {
			"101:3": {"id":1003,"report_id":101,"version":3,"status":"submitted","title":"甲月报","body_markdown":"完成 A","parse_status":"ready"},
			"102:2": {"id":1004,"report_id":102,"version":2,"status":"reviewed","title":"乙月报","body_markdown":"完成 B","parse_status":"ready"}
		}
	}`)
	suite := fixtureServerTestSuite(
		[]Case{{ID: "C01", Role: "captain", AllowedTools: []string{"get_report_materials"}, Fixture: map[string]string{"get_report_materials": "team"}}},
		map[string]map[string]json.RawMessage{"get_report_materials": {"team": fixture}},
	)
	server := startFixtureServer(t, suite)
	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	client := fixtureBusinessClient(t, server)
	ctx := fixtureCaseContext("C01", "captain")

	submitted, err := client.ListReports(ctx, 31, "submitted")
	if err != nil || len(submitted) != 1 || submitted[0].ID != 101 {
		t.Fatalf("submitted reports = %+v, err=%v", submitted, err)
	}
	reviewed, err := client.ListReports(ctx, 31, "reviewed")
	if err != nil || len(reviewed) != 1 || reviewed[0].ID != 102 {
		t.Fatalf("reviewed reports = %+v, err=%v", reviewed, err)
	}
	version, err := client.GetReportVersion(ctx, 102, 2)
	if err != nil || version.ID != 1004 || version.Title != "乙月报" {
		t.Fatalf("GetReportVersion() = %+v, err=%v", version, err)
	}

	ledger := server.Ledger("C01")
	if len(ledger.Requests) != 3 || ledger.ToolCalls["get_report_materials"] != 3 {
		t.Fatalf("report ledger = %+v", ledger)
	}
	for _, request := range ledger.Requests {
		if request.FixtureVariant != "team" || !request.Succeeded {
			t.Fatalf("unexpected report request: %+v", request)
		}
	}
}

func TestFixtureServerCountsWriteAttemptsSuccessesAndIdempotencyKeys(t *testing.T) {
	suite := fixtureServerTestSuite(
		[]Case{{ID: "M02", Role: "member", WriteEnabled: true, AllowedTools: []string{"clock_in"}}},
		map[string]map[string]json.RawMessage{
			"clock_in":  {"default": json.RawMessage(`{"id":801,"user_id":7,"created":true}`)},
			"clock_out": {"default": json.RawMessage(`{"id":802,"user_id":7,"created":true}`)},
		},
	)
	server := startFixtureServer(t, suite)
	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	client := fixtureBusinessClient(t, server)
	ctx := fixtureCaseContext("M02", "member")

	if _, err := client.ClockIn(ctx, "agent:eval-m02-1:clock-in"); err != nil {
		t.Fatalf("ClockIn() error = %v", err)
	}
	if _, err := client.ClockOut(ctx, "agent:eval-m02-1:clock-out"); err == nil {
		t.Fatal("ClockOut() unexpectedly succeeded on a non-allowed route")
	}

	ledger := server.Ledger("M02")
	if ledger.WriteAttempts != 2 || ledger.WriteSucceeded != 1 {
		t.Fatalf("write counts = attempts %d, succeeded %d", ledger.WriteAttempts, ledger.WriteSucceeded)
	}
	if len(ledger.IdempotencyKeys) != 2 || ledger.IdempotencyKeys[0] != "agent:eval-m02-1:clock-in" || ledger.IdempotencyKeys[1] != "agent:eval-m02-1:clock-out" {
		t.Fatalf("idempotency keys = %v", ledger.IdempotencyKeys)
	}
	if len(ledger.Requests) != 2 || ledger.Requests[1].StatusCode != http.StatusForbidden || ledger.Requests[1].Succeeded {
		t.Fatalf("requests = %+v", ledger.Requests)
	}

	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	reset := server.Ledger("M02")
	if len(reset.Requests) != 0 || reset.WriteAttempts != 0 || reset.WriteSucceeded != 0 {
		t.Fatalf("BeginCase did not reset ledger: %+v", reset)
	}
}

func TestFixtureServerRejectsTeamQueryThatDoesNotMatchVariant(t *testing.T) {
	suite := fixtureServerTestSuite(
		[]Case{{ID: "C02", Role: "captain", AllowedTools: []string{"query_team_members"}, Fixture: map[string]string{"query_team_members": "missing"}}},
		map[string]map[string]json.RawMessage{
			"query_team_members": {
				"missing": json.RawMessage(`{"period":"month","period_id":31,"report_status":"missing","limit":100,"members":[]}`),
			},
		},
	)
	server := startFixtureServer(t, suite)
	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	client := fixtureBusinessClient(t, server)
	ctx := fixtureCaseContext("C02", "captain")

	if _, err := client.QueryTeam(ctx, "month", 999, nil, "missing", 0); err == nil {
		t.Fatal("QueryTeam() unexpectedly accepted the wrong period_id")
	}
	result, err := client.QueryTeam(ctx, "month", 31, nil, "missing", 0)
	if err != nil || result.PeriodID != 31 {
		t.Fatalf("QueryTeam() = %+v, err=%v", result, err)
	}
	ledger := server.Ledger("C02")
	if len(ledger.Requests) != 2 || ledger.Requests[0].StatusCode != http.StatusBadRequest || !ledger.Requests[1].Succeeded {
		t.Fatalf("team query ledger = %+v", ledger)
	}
}

func TestFixtureServerRejectsReportPeriodThatDoesNotMatchVariant(t *testing.T) {
	fixture := json.RawMessage(`{
		"period_id":31,
		"reports":{"submitted":[],"reviewed":[]},
		"versions":{}
	}`)
	suite := fixtureServerTestSuite(
		[]Case{{ID: "M08", Role: "member", AllowedTools: []string{"get_report_materials"}, Fixture: map[string]string{"get_report_materials": "empty"}}},
		map[string]map[string]json.RawMessage{"get_report_materials": {"empty": fixture}},
	)
	server := startFixtureServer(t, suite)
	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	client := fixtureBusinessClient(t, server)
	ctx := fixtureCaseContext("M08", "member")

	if _, err := client.ListReports(ctx, 999, "submitted"); err == nil {
		t.Fatal("ListReports() unexpectedly accepted the wrong period_id for an empty fixture")
	}
	reports, err := client.ListReports(ctx, 31, "submitted")
	if err != nil || len(reports) != 0 {
		t.Fatalf("ListReports() = %+v, err=%v", reports, err)
	}
}

func TestFixtureServerValidatesAnnouncementDraftBodyExactly(t *testing.T) {
	fixture := json.RawMessage(`{
		"id":501,"status":"draft","title":"九月月报提醒",
		"content":"请尚未提交九月月报的成员于截止时间前完成。","pinned":false,
		"source_json":"{\"generated_by\":\"agent\",\"note\":\"基于未提交名单生成\"}",
		"recipient_ids":[12],"email_requested":false,"created_by":9
	}`)
	suite := fixtureServerTestSuite(
		[]Case{{ID: "C09", Role: "captain", WriteEnabled: true, AllowedTools: []string{"draft_announcement"}}},
		map[string]map[string]json.RawMessage{"draft_announcement": {"default": fixture}},
	)
	server := startFixtureServer(t, suite)
	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	client := fixtureBusinessClient(t, server)
	ctx := fixtureCaseContext("C09", "captain")
	want := business.AnnouncementDraftRequest{
		Title: "九月月报提醒", Content: "请尚未提交九月月报的成员于截止时间前完成。",
		Source: map[string]any{"generated_by": "agent", "note": "基于未提交名单生成"}, RecipientIDs: []int64{12},
	}
	if _, err := client.CreateAnnouncementDraft(ctx, want); err != nil {
		t.Fatalf("CreateAnnouncementDraft(valid) error = %v", err)
	}
	wrong := want
	wrong.RecipientIDs = []int64{99}
	if _, err := client.CreateAnnouncementDraft(ctx, wrong); err == nil {
		t.Fatal("CreateAnnouncementDraft() accepted a different recipient with the same count")
	}
	wrong = want
	wrong.Title = "九月报告提醒"
	if _, err := client.CreateAnnouncementDraft(ctx, wrong); err == nil {
		t.Fatal("CreateAnnouncementDraft() accepted different text with the same length")
	}
}

func TestFixtureServerRejectsKnowledgeQueryThatDoesNotMatchVariant(t *testing.T) {
	suite := fixtureServerTestSuite(
		[]Case{{ID: "M09", Role: "member", AllowedTools: []string{"search_knowledge"}, Fixture: map[string]string{"search_knowledge": "checkin"}}},
		map[string]map[string]json.RawMessage{
			"search_knowledge": {"checkin": json.RawMessage(`[{"id":301,"name":"打卡规则"}]`)},
		},
	)
	suite.Fixtures.RequestRules = map[string]map[string]FixtureRequestRule{
		"search_knowledge": {"checkin": {QueryTerms: []string{"打卡", "异常"}, MinQueryTermMatches: 1}},
	}
	server := startFixtureServer(t, suite)
	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}
	client := fixtureBusinessClient(t, server)
	ctx := fixtureCaseContext("M09", "member")

	if _, err := client.SearchKnowledge(ctx, "量子咖啡"); err == nil {
		t.Fatal("SearchKnowledge() unexpectedly accepted an unrelated query")
	}
	items, err := client.SearchKnowledge(ctx, "打卡异常怎么处理")
	if err != nil || len(items) != 1 || items[0].ID != 301 {
		t.Fatalf("SearchKnowledge() = %+v, err=%v", items, err)
	}
}

func TestFixtureServerRejectsUnknownTokenAndUnknownRoute(t *testing.T) {
	suite := fixtureServerTestSuite(
		[]Case{{ID: "M03", Role: "member", AllowedTools: []string{"get_checkin_status"}}},
		map[string]map[string]json.RawMessage{"get_checkin_status": {"default": json.RawMessage(`{"status":"working"}`)}},
	)
	server := startFixtureServer(t, suite)
	if err := server.BeginCase(suite.Cases[0]); err != nil {
		t.Fatal(err)
	}

	for _, token := range []string{"", "Bearer eval/M99", "Bearer wrong/M03"} {
		req, err := http.NewRequest(http.MethodGet, server.URL()+"/api/checkin/status", nil)
		if err != nil {
			t.Fatal(err)
		}
		if token != "" {
			req.Header.Set("Authorization", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("token %q status = %d, want 401", token, resp.StatusCode)
		}
	}

	req, err := http.NewRequest(http.MethodGet, server.URL()+"/api/not-a-route", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer eval/M03")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown route status = %d, want 404", resp.StatusCode)
	}
	ledger := server.Ledger("M03")
	if len(ledger.Requests) != 1 || ledger.Requests[0].StatusCode != http.StatusNotFound {
		t.Fatalf("unknown route ledger = %+v", ledger)
	}
}

func TestFixtureRouteForPathCoversBusinessClientContract(t *testing.T) {
	tests := []struct {
		path       string
		method     string
		tool       string
		write      bool
		reportList bool
		versionKey string
	}{
		{path: "/api/checkin/status", method: http.MethodGet, tool: "get_checkin_status"},
		{path: "/api/checkin/me/work-hours", method: http.MethodGet, tool: "get_my_work_hours"},
		{path: "/api/checkin/records", method: http.MethodGet, tool: "get_my_checkin_history"},
		{path: "/api/checkin/requirements", method: http.MethodGet, tool: "get_checkin_requirements"},
		{path: "/api/checkin/clock-in", method: http.MethodPost, tool: "clock_in", write: true},
		{path: "/api/checkin/clock-out", method: http.MethodPost, tool: "clock_out", write: true},
		{path: "/api/knowledge/search", method: http.MethodGet, tool: "search_knowledge"},
		{path: "/api/knowledge/recommendations", method: http.MethodGet, tool: "recommend_knowledge"},
		{path: "/api/checkin/stats", method: http.MethodGet, tool: "get_checkin_stats"},
		{path: "/api/analysis/team", method: http.MethodGet, tool: "query_team_members"},
		{path: "/api/report-periods", method: http.MethodGet, tool: "list_report_periods"},
		{path: "/api/reports", method: http.MethodGet, tool: "get_report_materials", reportList: true},
		{path: "/api/reports/101/versions/3", method: http.MethodGet, tool: "get_report_materials", versionKey: "101:3"},
		{path: "/api/announcement-drafts", method: http.MethodPost, tool: "draft_announcement", write: true},
	}
	for _, test := range tests {
		t.Run(test.tool+test.path, func(t *testing.T) {
			route, ok := fixtureRouteForPath(test.path)
			if !ok {
				t.Fatalf("fixtureRouteForPath(%q) was not found", test.path)
			}
			if route.tool != test.tool || route.method != test.method || route.write != test.write || route.reportList != test.reportList || route.versionKey != test.versionKey {
				t.Fatalf("fixtureRouteForPath(%q) = %+v", test.path, route)
			}
		})
	}
	for _, path := range []string{
		"/api/reports/101/versions/0",
		"/api/reports/not-an-id/versions/3",
		"/api/reports/101/versions/3/",
	} {
		if _, ok := fixtureRouteForPath(path); ok {
			t.Fatalf("invalid report path %q unexpectedly matched", path)
		}
	}
}

func fixtureServerTestSuite(cases []Case, tools map[string]map[string]json.RawMessage) *Suite {
	return &Suite{Cases: cases, Fixtures: BusinessFixtures{Version: 1, Tools: tools}}
}

func startFixtureServer(t *testing.T, suite *Suite) *FixtureServer {
	t.Helper()
	server, err := NewFixtureServer(suite)
	if err != nil {
		t.Fatalf("NewFixtureServer() error = %v", err)
	}
	t.Cleanup(server.Close)
	return server
}

func fixtureBusinessClient(t *testing.T, server *FixtureServer) *business.Client {
	t.Helper()
	client, err := business.NewClient(server.URL(), FixtureInternalSecret, time.Second)
	if err != nil {
		t.Fatalf("business.NewClient() error = %v", err)
	}
	return client
}

func fixtureCaseContext(caseID, role string) context.Context {
	return business.WithRequestContext(context.Background(), business.RequestContext{
		Authorization: "Bearer eval/" + caseID,
		UserID:        7,
		Role:          role,
		RequestID:     "eval-" + caseID,
	})
}

func TestValidEvaluationIdempotencyKey(t *testing.T) {
	if !validEvaluationIdempotencyKey("M15", "clock-in", "agent:eval-m15-2:clock-in") {
		t.Fatal("production evaluation idempotency key was rejected")
	}
	for _, value := range []string{"", "eval-m15-2:clock-in", "agent:eval-m15-0:clock-in", "agent:eval-m15-2:clock-out", "agent:eval-m16-2:clock-in"} {
		if validEvaluationIdempotencyKey("M15", "clock-in", value) {
			t.Fatalf("invalid idempotency key %q was accepted", value)
		}
	}
}
