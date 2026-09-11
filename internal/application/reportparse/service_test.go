package reportparse

import (
	"strings"
	"testing"
)

func TestParseRecognizesSectionsLinksAndLocations(t *testing.T) {
	result := Parse("# 2026 年 8 月月报\n\n## 本月成果\n完成接口治理，见 https://example.com/a。\n\n## 问题\n测试环境不足\n\n## 下月计划\n补齐迁移。\n")
	if result.Status != ParseReady || result.Report == nil {
		t.Fatalf("parse failed: %+v", result)
	}
	if len(result.Report.Sections) != 4 {
		t.Fatalf("section count = %d, want 4", len(result.Report.Sections))
	}
	if result.Report.Sections[1].Kind != SectionOutcome || result.Report.Sections[2].Kind != SectionIssues || result.Report.Sections[3].Kind != SectionPlan {
		t.Fatalf("section kinds were not classified: %+v", result.Report.Sections)
	}
	if len(result.Report.Links) != 1 || result.Report.Links[0].Line != 4 || result.Report.Links[0].Section != "本月成果" || result.Report.Links[0].URL != "https://example.com/a" {
		t.Fatalf("link location was not preserved: %+v", result.Report.Links)
	}
}

func TestParseRejectsInvalidOrOversizedBodies(t *testing.T) {
	if result := Parse(string([]byte{0xff})); result.Status != ParseFailed {
		t.Fatalf("invalid UTF-8 was accepted: %+v", result)
	}
	if result := Parse(strings.Repeat("a", MaxBodyRunes+1)); result.Status != ParseFailed {
		t.Fatalf("oversized body was accepted: %+v", result)
	}
}

func TestParseWarnsWhenNoStandardHeadingsExist(t *testing.T) {
	result := Parse("本月完成了接口整理，但下月还需要补充测试。")
	if result.Status != ParseReady || len(result.Report.Warnings) == 0 {
		t.Fatalf("missing heading warning not returned: %+v", result)
	}
}
