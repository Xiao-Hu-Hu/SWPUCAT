package tools

import (
	"SWPUCAT/internal/agent/business"
	"encoding/json"
	"strings"
	"testing"
)

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
