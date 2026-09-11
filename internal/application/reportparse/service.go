package reportparse

import (
	"errors"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	ParserVersion   = "markdown-v1"
	MaxBodyRunes    = 200000
	MaxSections     = 32
	MaxSectionRunes = 50000
)

var (
	ErrInvalidBody = errors.New("report body is invalid")
	urlPattern     = regexp.MustCompile(`https?://[^\s)\]}>]+`)
)

type SectionKind string

const (
	SectionOther   SectionKind = "other"
	SectionOutcome SectionKind = "outcomes"
	SectionIssues  SectionKind = "issues"
	SectionPlan    SectionKind = "next_plan"
)

type Section struct {
	Kind      SectionKind `json:"kind"`
	Heading   string      `json:"heading"`
	Content   string      `json:"content"`
	StartLine int         `json:"start_line"`
	EndLine   int         `json:"end_line"`
}

type Link struct {
	URL     string `json:"url"`
	Line    int    `json:"line"`
	Section string `json:"section,omitempty"`
}

type ParsedReport struct {
	ParserVersion string    `json:"parser_version"`
	Sections      []Section `json:"sections"`
	Links         []Link    `json:"links"`
	Warnings      []string  `json:"warnings,omitempty"`
}

type Result struct {
	Status ParseStatus   `json:"status"`
	Error  string        `json:"error,omitempty"`
	Report *ParsedReport `json:"report,omitempty"`
}

type ParseStatus string

const (
	ParseReady  ParseStatus = "ready"
	ParseFailed ParseStatus = "failed"
)

var headingPattern = regexp.MustCompile(`^\s{0,3}#{1,6}\s+(.+?)\s*#*\s*$`)

// Parse is deterministic and side-effect free. It extracts sections and
// evidence locations without interpreting report prose as instructions.
func Parse(body string) Result {
	if !utf8.ValidString(body) || strings.TrimSpace(body) == "" || utf8.RuneCountInString(body) > MaxBodyRunes {
		return Result{Status: ParseFailed, Error: ErrInvalidBody.Error()}
	}
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	parsed := &ParsedReport{ParserVersion: ParserVersion, Sections: make([]Section, 0), Links: make([]Link, 0)}
	current := -1
	for lineIndex, line := range lines {
		lineNo := lineIndex + 1
		match := headingPattern.FindStringSubmatch(line)
		if len(match) == 2 {
			if current >= 0 {
				parsed.Sections[current].EndLine = lineNo - 1
			}
			if len(parsed.Sections) >= MaxSections {
				return Result{Status: ParseFailed, Error: "report contains too many sections"}
			}
			heading := strings.TrimSpace(match[1])
			parsed.Sections = append(parsed.Sections, Section{Kind: classifyHeading(heading), Heading: heading, StartLine: lineNo + 1, EndLine: lineNo})
			current = len(parsed.Sections) - 1
		}
		if current >= 0 && lineIndex >= parsed.Sections[current].StartLine-1 {
			parsed.Sections[current].Content += line + "\n"
			if utf8.RuneCountInString(parsed.Sections[current].Content) > MaxSectionRunes {
				return Result{Status: ParseFailed, Error: "report section is too long"}
			}
		}
		for _, rawURL := range urlPattern.FindAllString(line, -1) {
			parsed.Links = append(parsed.Links, Link{URL: strings.TrimRight(rawURL, ".,;，。；"), Line: lineNo, Section: sectionHeading(parsed.Sections, current)})
		}
	}
	if current >= 0 {
		parsed.Sections[current].EndLine = len(lines)
	}
	for i := range parsed.Sections {
		parsed.Sections[i].Content = strings.TrimSpace(parsed.Sections[i].Content)
		if parsed.Sections[i].Content == "" {
			parsed.Warnings = append(parsed.Warnings, "章节“"+parsed.Sections[i].Heading+"”没有正文")
		}
	}
	if len(parsed.Sections) == 0 {
		parsed.Warnings = append(parsed.Warnings, "未识别 Markdown 章节标题，建议使用成果、问题和下月计划标题")
	}
	return Result{Status: ParseReady, Report: parsed}
}

func classifyHeading(heading string) SectionKind {
	compact := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(heading)), " ", "")
	for _, candidate := range []string{"成果", "本月成果", "完成情况", "工作内容", "完成工作"} {
		if strings.Contains(compact, strings.ToLower(candidate)) {
			return SectionOutcome
		}
	}
	for _, candidate := range []string{"问题", "困难", "风险", "阻塞"} {
		if strings.Contains(compact, candidate) {
			return SectionIssues
		}
	}
	for _, candidate := range []string{"下月计划", "下一步", "计划", "后续安排"} {
		if strings.Contains(compact, candidate) {
			return SectionPlan
		}
	}
	return SectionOther
}

func sectionHeading(sections []Section, index int) string {
	if index < 0 || index >= len(sections) {
		return ""
	}
	return sections[index].Heading
}
