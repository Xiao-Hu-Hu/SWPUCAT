package business

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCreateAnnouncementDraftSendsJSONAndIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/announcement-drafts" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("X-Agent-Internal-Secret") != "secret" {
			t.Fatalf("identity headers were not forwarded")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("content type = %q", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		var request AnnouncementDraftRequest
		if err := json.Unmarshal(body, &request); err != nil || request.Title != "公告" || request.EmailRequested {
			t.Fatalf("unexpected request body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"created","data":{"id":5,"status":"draft","title":"公告","content":"正文"}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRequestContext(context.Background(), RequestContext{Authorization: "Bearer token"})
	draft, err := client.CreateAnnouncementDraft(ctx, AnnouncementDraftRequest{Title: "公告", Content: "正文"})
	if err != nil || draft.ID != 5 || draft.Status != "draft" {
		t.Fatalf("CreateAnnouncementDraft() = %+v, err=%v", draft, err)
	}
}

func TestClientForwardsIdentityAndIdempotencyKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer access-token" {
			t.Errorf("Authorization header = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("X-Agent-Internal-Secret") != "internal-secret" {
			t.Errorf("internal secret header = %q", r.Header.Get("X-Agent-Internal-Secret"))
		}
		if r.Header.Get("Idempotency-Key") != "agent:req-1:clock-in" {
			t.Errorf("Idempotency-Key header = %q", r.Header.Get("Idempotency-Key"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"id":7}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "internal-secret", time.Second)
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	ctx := WithRequestContext(context.Background(), RequestContext{Authorization: "Bearer access-token"})
	if _, err := client.ClockIn(ctx, "agent:req-1:clock-in"); err != nil {
		t.Fatalf("ClockIn() error = %v", err)
	}
}

func TestClientReadsOnlyAuthenticatedMembersWorkHours(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/checkin/me/work-hours" || r.URL.RawQuery != "" {
			t.Fatalf("unexpected request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer member-token" || r.Header.Get("X-Agent-Internal-Secret") != "secret" {
			t.Fatalf("identity headers were not forwarded")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"as_of_date":"2026-09-17","user_id":7,"nickname":"成员甲","week":{"starts_on":"2026-09-14","ends_on":"2026-09-17","total_minutes":240,"total_hours":4,"record_days":2,"attendance_state":"normal","anomalies":[]},"month":{"starts_on":"2026-09-01","ends_on":"2026-09-17","total_minutes":960,"total_hours":16,"record_days":8,"attendance_state":"normal","anomalies":[]},"weekly_requirement":{"grade":3,"required_minutes":360,"required_hours":6,"is_met":false,"shortfall_minutes":120,"shortfall_hours":2,"status":"below_target"}}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRequestContext(context.Background(), RequestContext{Authorization: "Bearer member-token", UserID: 7, Role: "member"})
	result, err := client.GetMyWorkHours(ctx)
	if err != nil {
		t.Fatalf("GetMyWorkHours() error = %v", err)
	}
	if result.UserID != 7 || result.Week.TotalMinutes != 240 || result.Month.TotalMinutes != 960 {
		t.Fatalf("GetMyWorkHours() = %+v", result)
	}
	if result.WeeklyRequirement.IsMet == nil || *result.WeeklyRequirement.IsMet || result.WeeklyRequirement.ShortfallMinutes == nil || *result.WeeklyRequirement.ShortfallMinutes != 120 {
		t.Fatalf("weekly requirement = %+v", result.WeeklyRequirement)
	}
}

func TestBuildKnowledgeSourcesOmitsStorageDetailsAndBoundsContent(t *testing.T) {
	items := make([]KnowledgeItem, 0, 10)
	for i := 0; i < 10; i++ {
		items = append(items, KnowledgeItem{
			ID:           int64(i + 1),
			Type:         "file",
			Name:         "guide",
			Description:  string(make([]rune, 5000)),
			CategoryName: "流程",
			Status:       "approved",
		})
	}
	sources := BuildKnowledgeSources(items)
	if len(sources) != 8 {
		t.Fatalf("source count = %d, want 8", len(sources))
	}
	if got := len([]rune(sources[0].Description)); got != 4001 {
		t.Fatalf("description rune count = %d, want 4001 including ellipsis", got)
	}
	data, err := json.Marshal(sources[0])
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || containsJSONField(data, "file_key") {
		t.Fatalf("source contains storage detail: %s", data)
	}
}

func TestClientReadsReportMaterialsThroughProtectedAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/reports":
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":[{"id":11,"period_id":4,"author_id":7,"author_name":"成员","status":"submitted","current_version":2}]}`))
		case "/api/reports/11/versions/2":
			_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"id":21,"report_id":11,"version":2,"status":"submitted","title":"月报","body_markdown":"## 成果\\n完成","parse_status":"ready","parsed_json":{"sections":[]}}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	ctx := WithRequestContext(context.Background(), RequestContext{Authorization: "Bearer token"})
	reports, err := client.ListReports(ctx, 4, "submitted")
	if err != nil || len(reports) != 1 || reports[0].CurrentVersion != 2 {
		t.Fatalf("ListReports() = %+v, err=%v", reports, err)
	}
	version, err := client.GetReportVersion(ctx, 11, 2)
	if err != nil || version.ParseStatus != "ready" || version.ParsedJSON == nil {
		t.Fatalf("GetReportVersion() = %+v, err=%v", version, err)
	}
}

func TestKnowledgeSourceCollectorDeduplicatesAndReturnsExcerpts(t *testing.T) {
	collector := NewKnowledgeSourceCollector()
	items := []KnowledgeItem{{ID: 7, Name: "guide", Description: string(make([]rune, 800))}}
	collector.Add(items)
	collector.Add(items)
	sources := collector.List()
	if len(sources) != 1 {
		t.Fatalf("source count = %d, want 1", len(sources))
	}
	if got := len([]rune(sources[0].Description)); got != 601 {
		t.Fatalf("excerpt rune count = %d, want 601 including ellipsis", got)
	}
}

func containsJSONField(data []byte, field string) bool {
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		return false
	}
	_, ok := value[field]
	return ok
}
