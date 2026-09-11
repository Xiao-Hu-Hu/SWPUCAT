package tools

import (
	"SWPUCAT/internal/agent/business"
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
