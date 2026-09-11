package reportmigration

import (
	"regexp"
	"sort"
	"strings"
)

type LegacyKnowledgeItem struct {
	ID           int64  `json:"id"`
	Type         string `json:"type"`
	Name         string `json:"name"`
	Description  string `json:"description"`
	FileKey      string `json:"file_key"`
	CategoryID   int64  `json:"category_id"`
	CategoryName string `json:"category_name"`
	UploaderID   int64  `json:"uploader_id"`
	UploaderName string `json:"uploader_name"`
}

type Decision string

const (
	DecisionSuggested   Decision = "suggested"
	DecisionNeedsReview Decision = "needs_review"
	DecisionExcluded    Decision = "excluded"
)

type Candidate struct {
	KnowledgeItemID     int64    `json:"knowledge_item_id"`
	CategoryID          int64    `json:"category_id"`
	CategoryName        string   `json:"category_name"`
	SuggestedPeriodKey  string   `json:"suggested_period_key,omitempty"`
	SuggestedAuthorID   int64    `json:"suggested_author_id,omitempty"`
	SuggestedAuthorName string   `json:"suggested_author_name,omitempty"`
	Decision            Decision `json:"decision"`
	Confidence          float64  `json:"confidence"`
	Reason              string   `json:"reason"`
}

type DryRunSummary struct {
	Total       int `json:"total"`
	Suggested   int `json:"suggested"`
	NeedsReview int `json:"needs_review"`
	Excluded    int `json:"excluded"`
}

type DryRunResult struct {
	Candidates []Candidate   `json:"candidates"`
	Summary    DryRunSummary `json:"summary"`
}

var (
	periodDashPattern    = regexp.MustCompile(`(?i)(20\d{2})[-_./](0?[1-9]|1[0-2])`)
	periodCNPattern      = regexp.MustCompile(`(20\d{2})年(0?[1-9]|1[0-2])月`)
	periodCompactPattern = regexp.MustCompile(`(?i)(20\d{2})(0[1-9]|1[0-2])月`)
	reportNamePattern    = regexp.MustCompile(`(?i)(月报|monthly|monthly-report|工作报告)`)
)

// BuildDryRun is deterministic and side-effect free. It intentionally makes
// uncertain records visible for manual review instead of guessing a month or
// silently moving an old file into a formal report.
func BuildDryRun(items []LegacyKnowledgeItem) DryRunResult {
	candidates := make([]Candidate, 0, len(items))
	for _, item := range items {
		candidate := classify(item)
		candidates = append(candidates, candidate)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].SuggestedPeriodKey != candidates[j].SuggestedPeriodKey {
			return candidates[i].SuggestedPeriodKey < candidates[j].SuggestedPeriodKey
		}
		return candidates[i].KnowledgeItemID < candidates[j].KnowledgeItemID
	})
	result := DryRunResult{Candidates: candidates}
	for _, candidate := range candidates {
		result.Summary.Total++
		switch candidate.Decision {
		case DecisionSuggested:
			result.Summary.Suggested++
		case DecisionNeedsReview:
			result.Summary.NeedsReview++
		case DecisionExcluded:
			result.Summary.Excluded++
		}
	}
	return result
}

func classify(item LegacyKnowledgeItem) Candidate {
	candidate := Candidate{
		KnowledgeItemID:     item.ID,
		CategoryID:          item.CategoryID,
		CategoryName:        strings.TrimSpace(item.CategoryName),
		SuggestedAuthorID:   item.UploaderID,
		SuggestedAuthorName: strings.TrimSpace(item.UploaderName),
		Decision:            DecisionExcluded,
		Reason:              "not identified as a monthly report",
	}
	if item.ID <= 0 || item.Type != "file" {
		return candidate
	}
	text := strings.Join([]string{item.Name, item.Description, item.FileKey, item.CategoryName}, " ")
	period := inferPeriodKey(text)
	isReport := reportNamePattern.MatchString(text) || strings.Contains(strings.ToLower(item.CategoryName), "report")
	if !isReport {
		return candidate
	}
	if period == "" {
		candidate.Decision = DecisionNeedsReview
		candidate.Confidence = 0.25
		candidate.Reason = "report-like item has no unambiguous YYYY-MM or YYYY年MM月 marker"
		return candidate
	}
	candidate.SuggestedPeriodKey = period
	candidate.Confidence = 0.85
	candidate.Decision = DecisionSuggested
	candidate.Reason = "period marker and report-like name/category matched; author is uploader snapshot"
	return candidate
}

func inferPeriodKey(text string) string {
	for _, pattern := range []*regexp.Regexp{periodDashPattern, periodCNPattern, periodCompactPattern} {
		matches := pattern.FindStringSubmatch(text)
		if len(matches) != 3 {
			continue
		}
		month := matches[2]
		if len(month) == 1 {
			month = "0" + month
		}
		return matches[1] + "-" + month
	}
	return ""
}
