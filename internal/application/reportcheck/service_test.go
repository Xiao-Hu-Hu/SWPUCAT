package reportcheck

import (
	reportparse "SWPUCAT/internal/application/reportparse"
	"testing"
)

func TestCheckReportsMissingSectionsAndVagueOutcome(t *testing.T) {
	parsed := reportparse.Parse("## 本月成果\n完成相关工作。\n")
	result := Check(parsed.Report)
	if result.Status != "ready" || len(result.Findings) < 3 {
		t.Fatalf("unexpected quality result: %+v", result)
	}
	if result.Findings[0].Kind != VagueExpression {
		t.Fatalf("first finding kind = %s, want vague_expression", result.Findings[0].Kind)
	}
}

func TestCheckDoesNotTreatMissingIssuesAsNoIssues(t *testing.T) {
	result := Check(&reportparse.ParsedReport{Sections: []reportparse.Section{{Kind: reportparse.SectionOutcome, Heading: "成果", Content: "完成接口", StartLine: 2, EndLine: 2}}})
	found := false
	for _, finding := range result.Findings {
		if finding.Message == "缺少问题章节" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing issues section was not reported: %+v", result.Findings)
	}
}
