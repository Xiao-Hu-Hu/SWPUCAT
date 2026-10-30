package reportmigration

import "testing"

func TestBuildDryRunSeparatesSuggestedUnknownAndExcludedItems(t *testing.T) {
	result := BuildDryRun([]LegacyKnowledgeItem{
		{ID: 3, Type: "file", Name: "2026-08 月报", CategoryName: "月报", UploaderID: 9, UploaderName: "成员"},
		{ID: 2, Type: "file", Name: "月报", CategoryName: "月报", UploaderID: 8},
		{ID: 1, Type: "file", Name: "Go 教程", CategoryName: "学习资料"},
		{ID: 4, Type: "link", Name: "2026-08 月报", CategoryName: "月报"},
	})
	if result.Summary.Total != 4 || result.Summary.Suggested != 1 || result.Summary.NeedsReview != 1 || result.Summary.Excluded != 2 {
		t.Fatalf("unexpected summary: %+v", result.Summary)
	}
	if result.Candidates[0].KnowledgeItemID != 1 || result.Candidates[1].KnowledgeItemID != 2 || result.Candidates[2].KnowledgeItemID != 4 || result.Candidates[3].KnowledgeItemID != 3 {
		t.Fatalf("candidates were not deterministically sorted: %+v", result.Candidates)
	}
	if result.Candidates[3].SuggestedPeriodKey != "2026-08" || result.Candidates[3].SuggestedAuthorID != 9 {
		t.Fatalf("suggested candidate lost period or author snapshot: %+v", result.Candidates[3])
	}
}

func TestBuildDryRunRecognizesChinesePeriodMarker(t *testing.T) {
	result := BuildDryRun([]LegacyKnowledgeItem{{ID: 5, Type: "file", Name: "2026年8月工作报告", CategoryName: "月报"}})
	if len(result.Candidates) != 1 || result.Candidates[0].SuggestedPeriodKey != "2026-08" {
		t.Fatalf("Chinese period marker was not recognized: %+v", result.Candidates)
	}
}
