package business

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

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
