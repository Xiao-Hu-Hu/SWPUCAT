package reportcheck

import (
	reportparse "SWPUCAT/internal/application/reportparse"
	"strings"
)

type FindingKind string

const (
	MissingContent       FindingKind = "missing_content"
	VagueExpression      FindingKind = "vague_expression"
	PossiblePlanAsOutcome FindingKind = "possible_plan_as_outcome"
)

type Finding struct {
	Index      int         `json:"index"`
	Kind       FindingKind `json:"kind"`
	Message    string      `json:"message"`
	Suggestion string      `json:"suggestion"`
	StartLine  int         `json:"start_line"`
	EndLine    int         `json:"end_line"`
	Original   string      `json:"original,omitempty"`
}

type Result struct {
	Status   string    `json:"status"`
	Findings []Finding `json:"findings"`
}

func Check(parsed *reportparse.ParsedReport) Result {
	if parsed == nil {
		return Result{Status: "failed", Findings: []Finding{{Index: 0, Kind: MissingContent, Message: "报告正文无法解析", Suggestion: "请检查 Markdown 编码和正文长度"}}}
	}
	result := Result{Status: "ready", Findings: make([]Finding, 0)}
	hasOutcome, hasIssues, hasPlan := false, false, false
	for _, section := range parsed.Sections {
		switch section.Kind {
		case reportparse.SectionOutcome:
			hasOutcome = true
			if strings.TrimSpace(section.Content) == "" {
				result.Findings = append(result.Findings, Finding{Kind: MissingContent, Message: "成果章节为空", Suggestion: "补充本月完成的具体事项、结果和证据", StartLine: section.StartLine, EndLine: section.EndLine, Original: section.Content})
			}
			if containsVaguePhrase(section.Content) {
				result.Findings = append(result.Findings, Finding{Kind: VagueExpression, Message: "成果表述较笼统", Suggestion: "补充具体对象、完成结果、影响或可核对链接", StartLine: section.StartLine, EndLine: section.EndLine, Original: section.Content})
			}
			if containsPlanMarker(section.Content) {
				result.Findings = append(result.Findings, Finding{Kind: PossiblePlanAsOutcome, Message: "成果章节包含计划性措辞", Suggestion: "确认这段是已完成事实；未完成内容请移动到计划章节", StartLine: section.StartLine, EndLine: section.EndLine, Original: section.Content})
			}
		case reportparse.SectionIssues:
			hasIssues = true
			if strings.TrimSpace(section.Content) == "" {
				result.Findings = append(result.Findings, Finding{Kind: MissingContent, Message: "问题章节为空", Suggestion: "填写遇到的问题；如果确实没有问题，请明确写出“暂无”及依据", StartLine: section.StartLine, EndLine: section.EndLine, Original: section.Content})
			}
		case reportparse.SectionPlan:
			hasPlan = true
			if strings.TrimSpace(section.Content) == "" {
				result.Findings = append(result.Findings, Finding{Kind: MissingContent, Message: "下月计划章节为空", Suggestion: "补充下一阶段目标、负责人或可验收结果", StartLine: section.StartLine, EndLine: section.EndLine, Original: section.Content})
			}
		}
	}
	if !hasOutcome {
		result.Findings = append(result.Findings, Finding{Kind: MissingContent, Message: "缺少成果章节", Suggestion: "增加“本月成果”或“完成情况”标题并填写事实"})
	}
	if !hasIssues {
		result.Findings = append(result.Findings, Finding{Kind: MissingContent, Message: "缺少问题章节", Suggestion: "增加“问题”标题；没有问题时明确说明暂无"})
	}
	if !hasPlan {
		result.Findings = append(result.Findings, Finding{Kind: MissingContent, Message: "缺少下月计划章节", Suggestion: "增加“下月计划”或“下一步”标题并填写计划"})
	}
	for i := range result.Findings {
		result.Findings[i].Index = i
	}
	return result
}

func containsVaguePhrase(content string) bool {
	for _, phrase := range []string{"完成相关工作", "推进项目", "优化系统", "参与项目", "做好工作", "按计划进行"} {
		if strings.Contains(content, phrase) {
			return true
		}
	}
	return false
}

func containsPlanMarker(content string) bool {
	for _, marker := range []string{"下月", "计划", "预计", "将完成", "准备"} {
		if strings.Contains(content, marker) {
			return true
		}
	}
	return false
}
