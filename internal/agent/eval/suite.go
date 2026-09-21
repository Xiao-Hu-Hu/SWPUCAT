package agenteval

import (
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const DefaultSuite = "v1"

var (
	//go:embed testdata/v1/*.json testdata/v1/README.md
	embeddedSuites embed.FS

	suiteNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	caseIDPattern    = regexp.MustCompile(`^(M|C|S)[0-9]{2}$`)
	reportRefPattern = regexp.MustCompile(`^\[报告#([0-9]+)-v([0-9]+)\]$`)
)

var allToolNames = []string{
	"get_checkin_status",
	"get_my_work_hours",
	"get_my_checkin_history",
	"get_checkin_requirements",
	"search_knowledge",
	"recommend_knowledge",
	"query_team_members",
	"get_checkin_stats",
	"list_report_periods",
	"get_report_materials",
	"get_report_roster",
	"list_pending_knowledge",
	"get_online_summary",
	"draft_announcement",
	"review_knowledge_item",
	"generate_invitation_code",
	"save_evaluation_draft",
	"clock_in",
	"clock_out",
}

var writeToolNames = map[string]struct{}{
	"clock_in":                 {},
	"clock_out":                {},
	"draft_announcement":       {},
	"review_knowledge_item":    {},
	"generate_invitation_code": {},
	"save_evaluation_draft":    {},
}

var memberForbiddenTools = map[string]struct{}{
	"query_team_members":       {},
	"get_report_roster":        {},
	"get_checkin_stats":        {},
	"list_pending_knowledge":   {},
	"draft_announcement":       {},
	"review_knowledge_item":    {},
	"generate_invitation_code": {},
	"save_evaluation_draft":    {},
}

var argumentProjectionKeys = map[string]map[string]struct{}{
	"get_checkin_status":       {},
	"get_my_work_hours":        {},
	"get_my_checkin_history":   stringSet([]string{"limit"}),
	"get_checkin_requirements": {},
	"search_knowledge":         stringSet([]string{"query_length"}),
	"recommend_knowledge":      stringSet([]string{"need_length"}),
	"query_team_members":       stringSet([]string{"period_id_provided", "min_hours", "limit", "period", "report_status"}),
	"get_checkin_stats":        stringSet([]string{"period"}),
	"list_report_periods":      stringSet([]string{"limit", "period_key", "status"}),
	"get_report_materials":     stringSet([]string{"period_id_provided", "offset", "limit", "scope"}),
	"get_report_roster":        stringSet([]string{"period_id_provided", "limit", "report_status"}),
	"list_pending_knowledge":   stringSet([]string{"limit"}),
	"get_online_summary":       {},
	"draft_announcement":       stringSet([]string{"title_length", "content_length", "source_note_length", "pinned", "recipient_count"}),
	"review_knowledge_item":    stringSet([]string{"item_id_provided", "decision", "reason_length"}),
	"generate_invitation_code": stringSet([]string{"type"}),
	"save_evaluation_draft":    stringSet([]string{"mode", "draft_id_provided", "snapshot_id_provided", "title_length", "summary_length", "audience"}),
	"clock_in":                 {},
	"clock_out":                {},
}

type Manifest struct {
	SchemaVersion     int            `json:"schema_version"`
	SuiteID           string         `json:"suite_id"`
	Description       string         `json:"description"`
	ExpectedCaseCount int            `json:"expected_case_count"`
	MinCases          int            `json:"min_cases"`
	MaxCases          int            `json:"max_cases"`
	CaseFiles         []string       `json:"case_files"`
	FixtureFile       string         `json:"fixture_file"`
	RoleCounts        map[string]int `json:"role_counts"`
	RequiredTools     []string       `json:"required_tools"`
	RequiredOutcomes  []string       `json:"required_outcomes"`
	RequiredTags      []string       `json:"required_tags"`
}

type HistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type KnowledgeContextItem struct {
	ID           int64   `json:"id"`
	Type         string  `json:"type,omitempty"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	URL          string  `json:"url,omitempty"`
	DownloadURL  string  `json:"download_url,omitempty"`
	CategoryName string  `json:"category_name,omitempty"`
	SourceKind   string  `json:"source_kind,omitempty"`
	Excerpt      string  `json:"excerpt,omitempty"`
	Score        float64 `json:"score,omitempty"`
	MatchMode    string  `json:"match_mode,omitempty"`
	CreatedAt    string  `json:"created_at,omitempty"`
}

// Case is intentionally declarative. required_tools is an ordered subsequence;
// every observed tool must also be in allowed_tools and no forbidden tool may
// run. tool_arguments contains the privacy-safe argument projections emitted by
// production telemetry, one entry per expected call to that tool.
type Case struct {
	ID                   string                      `json:"id"`
	Version              int                         `json:"version"`
	Priority             string                      `json:"priority"`
	Role                 string                      `json:"role"`
	Entrypoint           string                      `json:"entrypoint"`
	WriteEnabled         bool                        `json:"write_enabled"`
	Prompt               string                      `json:"prompt"`
	History              []HistoryMessage            `json:"history"`
	RollingSummary       string                      `json:"rolling_summary"`
	KnowledgeContext     []KnowledgeContextItem      `json:"knowledge_context"`
	Fixture              map[string]string           `json:"fixture"`
	ExpectedOutcome      string                      `json:"expected_outcome"`
	AllowedTools         []string                    `json:"allowed_tools"`
	RequiredTools        []string                    `json:"required_tools"`
	OptionalTools        []string                    `json:"optional_tools"`
	ForbiddenTools       []string                    `json:"forbidden_tools"`
	ToolArguments        map[string][]map[string]any `json:"tool_arguments"`
	IsWrite              bool                        `json:"is_write"`
	RequiresCitation     bool                        `json:"requires_citation"`
	CitationKind         string                      `json:"citation_kind"`
	ExpectedKnowledgeIDs []int64                     `json:"expected_knowledge_ids"`
	ExpectedReportRefs   []string                    `json:"expected_report_refs"`
	AnswerMustContain    []string                    `json:"answer_must_contain"`
	AnswerMustContainAny []string                    `json:"answer_must_contain_any"`
	AnswerMustNotContain []string                    `json:"answer_must_not_contain"`
	MaxToolCalls         int                         `json:"max_tool_calls"`
	Tags                 []string                    `json:"tags"`
	PrivacyCanaries      []string                    `json:"privacy_canaries"`
}

type BusinessFixtures struct {
	Version      int                                      `json:"version"`
	Tools        map[string]map[string]json.RawMessage    `json:"tools"`
	RequestRules map[string]map[string]FixtureRequestRule `json:"request_rules"`
}

type FixtureRequestRule struct {
	QueryTerms          []string `json:"query_terms"`
	MinQueryTermMatches int      `json:"min_query_term_matches"`
}

type Suite struct {
	Manifest Manifest
	Cases    []Case
	Fixtures BusinessFixtures
}

func ToolNames() []string {
	return append([]string(nil), allToolNames...)
}

func LoadEmbedded(name string) (*Suite, error) {
	if !suiteNamePattern.MatchString(name) {
		return nil, fmt.Errorf("invalid suite name %q", name)
	}
	return LoadFS(embeddedSuites, path.Join("testdata", name))
}

func LoadFS(source fs.FS, root string) (*Suite, error) {
	manifestPath := path.Join(root, "manifest.json")
	var manifest Manifest
	if err := decodeFile(source, manifestPath, &manifest); err != nil {
		return nil, fmt.Errorf("load manifest: %w", err)
	}
	suite := &Suite{Manifest: manifest}
	for _, filename := range manifest.CaseFiles {
		if !safeDataFilename(filename) {
			return nil, fmt.Errorf("manifest case file %q is unsafe", filename)
		}
		var cases []Case
		if err := decodeFile(source, path.Join(root, filename), &cases); err != nil {
			return nil, fmt.Errorf("load cases %s: %w", filename, err)
		}
		suite.Cases = append(suite.Cases, cases...)
	}
	if !safeDataFilename(manifest.FixtureFile) {
		return nil, fmt.Errorf("manifest fixture file %q is unsafe", manifest.FixtureFile)
	}
	if err := decodeFile(source, path.Join(root, manifest.FixtureFile), &suite.Fixtures); err != nil {
		return nil, fmt.Errorf("load fixtures: %w", err)
	}
	if err := suite.Validate(); err != nil {
		return nil, err
	}
	return suite, nil
}

func decodeFile(source fs.FS, filename string, target any) error {
	file, err := source.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values are not allowed")
		}
		return err
	}
	return nil
}

func safeDataFilename(filename string) bool {
	return filename != "" && path.Base(filename) == filename && strings.HasSuffix(filename, ".json")
}

type validationIssues []string

func (v validationIssues) Error() string {
	return "invalid agent evaluation suite:\n - " + strings.Join(v, "\n - ")
}

func (s *Suite) Validate() error {
	var issues validationIssues
	manifest := s.Manifest
	if manifest.SchemaVersion != 1 {
		issues = append(issues, fmt.Sprintf("schema_version=%d, want 1", manifest.SchemaVersion))
	}
	if strings.TrimSpace(manifest.SuiteID) == "" {
		issues = append(issues, "suite_id is required")
	}
	if strings.TrimSpace(manifest.Description) == "" {
		issues = append(issues, "description is required")
	}
	if manifest.MinCases < 40 || manifest.MaxCases > 60 || manifest.MinCases > manifest.MaxCases {
		issues = append(issues, "case bounds must stay within 40..60")
	}
	if len(s.Cases) != manifest.ExpectedCaseCount {
		issues = append(issues, fmt.Sprintf("case count=%d, manifest expects %d", len(s.Cases), manifest.ExpectedCaseCount))
	}
	if len(s.Cases) < manifest.MinCases || len(s.Cases) > manifest.MaxCases {
		issues = append(issues, fmt.Sprintf("case count=%d, want %d..%d", len(s.Cases), manifest.MinCases, manifest.MaxCases))
	}
	if s.Fixtures.Version != 1 {
		issues = append(issues, fmt.Sprintf("fixture version=%d, want 1", s.Fixtures.Version))
	}

	knownTools := stringSet(allToolNames)
	if !sameStringSet(manifest.RequiredTools, allToolNames) {
		issues = append(issues, "manifest required_tools must equal the production 18-tool contract")
	}
	if len(s.Fixtures.Tools) == 0 {
		issues = append(issues, "business fixtures are empty")
	}
	if !sameStringSet(manifest.RequiredOutcomes, []string{"support", "refuse", "clarify", "no_data"}) {
		issues = append(issues, "manifest required_outcomes must contain support, refuse, clarify, and no_data")
	}
	if !sameStringSet(mapKeys(manifest.RoleCounts), []string{"member", "captain", "super_admin"}) {
		issues = append(issues, "manifest role_counts must contain exactly member, captain, and super_admin")
	}
	for toolName, variants := range s.Fixtures.Tools {
		if _, ok := knownTools[toolName]; !ok {
			issues = append(issues, fmt.Sprintf("fixtures reference unknown tool %q", toolName))
		}
		if len(variants) == 0 {
			issues = append(issues, fmt.Sprintf("fixtures for %s are empty", toolName))
		}
		for name, data := range variants {
			if strings.TrimSpace(name) == "" || len(data) == 0 || !json.Valid(data) {
				issues = append(issues, fmt.Sprintf("fixture %s/%s is invalid", toolName, name))
			}
		}
	}
	for toolName, variants := range s.Fixtures.RequestRules {
		if _, ok := knownTools[toolName]; !ok {
			issues = append(issues, fmt.Sprintf("request rules reference unknown tool %q", toolName))
		}
		for variant, rule := range variants {
			if _, ok := s.Fixtures.Tools[toolName][variant]; !ok {
				issues = append(issues, fmt.Sprintf("request rule %s/%s has no matching fixture", toolName, variant))
			}
			if len(rule.QueryTerms) == 0 || hasEmptyOrDuplicate(rule.QueryTerms) || rule.MinQueryTermMatches < 1 || rule.MinQueryTermMatches > len(rule.QueryTerms) {
				issues = append(issues, fmt.Sprintf("request rule %s/%s has invalid query term requirements", toolName, variant))
			}
		}
	}

	roleCounts := make(map[string]int)
	outcomeCounts := make(map[string]int)
	tagCounts := make(map[string]int)
	requiredToolCounts := make(map[string]int)
	seenIDs := make(map[string]struct{})
	validRoles := stringSet([]string{"member", "captain", "super_admin"})
	validOutcomes := stringSet([]string{"support", "refuse", "clarify", "no_data"})
	validEntrypoints := stringSet([]string{"engine_direct", "public_api", "tool_direct"})

	for index := range s.Cases {
		item := &s.Cases[index]
		prefix := fmt.Sprintf("case[%d] %s", index, item.ID)
		if !caseIDPattern.MatchString(item.ID) {
			issues = append(issues, prefix+": id must match M01, C01, or S01 form")
		}
		if _, exists := seenIDs[item.ID]; exists {
			issues = append(issues, prefix+": duplicate id")
		}
		seenIDs[item.ID] = struct{}{}
		if item.Version != 1 {
			issues = append(issues, prefix+": version must be 1")
		}
		if item.Priority != "P1" {
			issues = append(issues, prefix+": priority must be P1")
		}
		if _, ok := validRoles[item.Role]; !ok {
			issues = append(issues, prefix+": invalid role")
		} else {
			roleCounts[item.Role]++
		}
		if !idMatchesRole(item.ID, item.Role) {
			issues = append(issues, prefix+": id prefix does not match role")
		}
		if _, ok := validEntrypoints[item.Entrypoint]; !ok {
			issues = append(issues, prefix+": invalid entrypoint")
		}
		prompt := strings.TrimSpace(item.Prompt)
		if prompt == "" || utf8.RuneCountInString(prompt) > 4000 {
			issues = append(issues, prefix+": prompt must contain 1..4000 runes")
		}
		if _, ok := validOutcomes[item.ExpectedOutcome]; !ok {
			issues = append(issues, prefix+": invalid expected_outcome")
		} else {
			outcomeCounts[item.ExpectedOutcome]++
		}
		if item.MaxToolCalls < 0 || item.MaxToolCalls > 8 {
			issues = append(issues, prefix+": max_tool_calls must be 0..8")
		}
		if len(item.Tags) == 0 || hasEmptyOrDuplicate(item.Tags) {
			issues = append(issues, prefix+": tags must be non-empty and unique")
		}
		for _, tag := range item.Tags {
			tagCounts[tag]++
		}

		allowed := validateToolList(&issues, prefix, "allowed_tools", item.AllowedTools, knownTools)
		required := validateToolList(&issues, prefix, "required_tools", item.RequiredTools, knownTools)
		optional := validateToolList(&issues, prefix, "optional_tools", item.OptionalTools, knownTools)
		forbidden := validateToolList(&issues, prefix, "forbidden_tools", item.ForbiddenTools, knownTools)
		for toolName := range required {
			requiredToolCounts[toolName]++
			if _, ok := allowed[toolName]; !ok {
				issues = append(issues, fmt.Sprintf("%s: required tool %s is not allowed", prefix, toolName))
			}
		}
		for toolName := range optional {
			if _, ok := allowed[toolName]; !ok {
				issues = append(issues, fmt.Sprintf("%s: optional tool %s is not allowed", prefix, toolName))
			}
		}
		for toolName := range forbidden {
			if _, ok := allowed[toolName]; ok {
				issues = append(issues, fmt.Sprintf("%s: tool %s is both allowed and forbidden", prefix, toolName))
			}
			if _, ok := required[toolName]; ok {
				issues = append(issues, fmt.Sprintf("%s: tool %s is both required and forbidden", prefix, toolName))
			}
		}
		expectedAllowed := allowedToolsForRole(item.Role, item.WriteEnabled)
		if !sameStringSet(item.AllowedTools, expectedAllowed) {
			issues = append(issues, prefix+": allowed_tools must equal the complete role/config capability set")
		}
		expectedForbidden := subtractStrings(allToolNames, expectedAllowed)
		if !sameStringSet(item.ForbiddenTools, expectedForbidden) {
			issues = append(issues, prefix+": forbidden_tools must be the complement of allowed_tools")
		}
		if item.MaxToolCalls < len(item.RequiredTools) {
			issues = append(issues, prefix+": max_tool_calls is smaller than required_tools")
		}
		if item.Role == "member" {
			for toolName := range memberForbiddenTools {
				if _, ok := allowed[toolName]; ok {
					issues = append(issues, fmt.Sprintf("%s: member cannot allow %s", prefix, toolName))
				}
				if _, ok := required[toolName]; ok {
					issues = append(issues, fmt.Sprintf("%s: member cannot require %s", prefix, toolName))
				}
			}
		}
		if !item.WriteEnabled {
			for toolName := range writeToolNames {
				if _, ok := allowed[toolName]; ok {
					issues = append(issues, fmt.Sprintf("%s: write-disabled case allows %s", prefix, toolName))
				}
				if _, ok := required[toolName]; ok {
					issues = append(issues, fmt.Sprintf("%s: write-disabled case requires %s", prefix, toolName))
				}
			}
		}
		if item.IsWrite {
			if !item.WriteEnabled || item.ExpectedOutcome != "support" || !containsWriteTool(item.RequiredTools) {
				issues = append(issues, prefix+": is_write requires an enabled, supported, required write tool")
			}
		}
		if item.ExpectedOutcome == "refuse" || item.ExpectedOutcome == "clarify" {
			if item.IsWrite {
				issues = append(issues, prefix+": refusal and clarification cases cannot expect writes")
			}
		}

		for toolName, variant := range item.Fixture {
			variants, ok := s.Fixtures.Tools[toolName]
			if !ok {
				issues = append(issues, fmt.Sprintf("%s: fixture references unknown tool %s", prefix, toolName))
				continue
			}
			if _, ok := variants[variant]; !ok {
				issues = append(issues, fmt.Sprintf("%s: fixture variant %s/%s is missing", prefix, toolName, variant))
			}
		}
		for toolName := range allowed {
			variants := s.Fixtures.Tools[toolName]
			variant := item.Fixture[toolName]
			if variant == "" {
				variant = "default"
			}
			if _, ok := variants[variant]; !ok {
				issues = append(issues, fmt.Sprintf("%s: allowed tool %s has no fixture variant %q", prefix, toolName, variant))
			}
		}
		for toolName, expected := range item.ToolArguments {
			if _, ok := allowed[toolName]; !ok {
				issues = append(issues, fmt.Sprintf("%s: tool_arguments references non-allowed tool %s", prefix, toolName))
			}
			if len(expected) == 0 {
				issues = append(issues, fmt.Sprintf("%s: tool_arguments for %s is empty", prefix, toolName))
			}
			keys := argumentProjectionKeys[toolName]
			for callIndex, projection := range expected {
				for key := range projection {
					if _, ok := keys[key]; !ok {
						issues = append(issues, fmt.Sprintf("%s: tool_arguments %s[%d] contains unsupported telemetry key %q", prefix, toolName, callIndex, key))
					}
				}
			}
		}
		for toolName := range required {
			if toolName == "search_knowledge" || toolName == "recommend_knowledge" {
				variant := item.Fixture[toolName]
				if variant == "" {
					variant = "default"
				}
				if _, ok := s.Fixtures.RequestRules[toolName][variant]; !ok {
					issues = append(issues, fmt.Sprintf("%s: required tool %s has no request rule for variant %q", prefix, toolName, variant))
				}
				continue
			}
			if _, hasArguments := item.ToolArguments[toolName]; toolAcceptsArguments(toolName) && !hasArguments {
				issues = append(issues, fmt.Sprintf("%s: required tool %s has no argument assertion", prefix, toolName))
			}
		}
		if len(item.AnswerMustContain) == 0 && len(item.AnswerMustContainAny) == 0 && !item.RequiresCitation {
			issues = append(issues, prefix+": answer has no deterministic positive assertion")
		}
		if item.Role == "member" {
			for _, projection := range item.ToolArguments["get_report_materials"] {
				if projection["scope"] == "team" {
					issues = append(issues, prefix+": member report scope cannot be team")
				}
			}
		}

		validateCitations(&issues, prefix, item)
		validateEvidenceFixtures(&issues, prefix, item, s.Fixtures)
		validateIdentityFixtures(&issues, prefix, item, s.Fixtures)
		validatePrivacyCanaries(&issues, prefix, item, s.Fixtures)
		for _, history := range item.History {
			if history.Role != "user" && history.Role != "assistant" {
				issues = append(issues, prefix+": history role must be user or assistant")
			}
			if strings.TrimSpace(history.Content) == "" {
				issues = append(issues, prefix+": history content is empty")
			}
		}
		for _, canary := range item.PrivacyCanaries {
			if strings.TrimSpace(canary) == "" {
				issues = append(issues, prefix+": privacy canary is empty")
			}
		}
	}

	for role, expected := range manifest.RoleCounts {
		if roleCounts[role] != expected {
			issues = append(issues, fmt.Sprintf("role %s has %d cases, want %d", role, roleCounts[role], expected))
		}
	}
	for _, outcome := range manifest.RequiredOutcomes {
		if outcomeCounts[outcome] == 0 {
			issues = append(issues, fmt.Sprintf("outcome %s is not covered", outcome))
		}
	}
	for _, tag := range manifest.RequiredTags {
		if tagCounts[tag] == 0 {
			issues = append(issues, fmt.Sprintf("tag %s is not covered", tag))
		}
	}
	for _, toolName := range allToolNames {
		if requiredToolCounts[toolName] == 0 {
			issues = append(issues, fmt.Sprintf("tool %s has no positive required-call case", toolName))
		}
	}
	if len(issues) > 0 {
		sort.Strings(issues)
		return issues
	}
	return nil
}

func toolAcceptsArguments(toolName string) bool {
	switch toolName {
	case "get_my_checkin_history", "query_team_members", "get_checkin_stats", "list_report_periods", "get_report_materials", "get_report_roster", "list_pending_knowledge", "draft_announcement", "review_knowledge_item", "generate_invitation_code", "save_evaluation_draft":
		return true
	default:
		return false
	}
}

func validateCitations(issues *validationIssues, prefix string, item *Case) {
	if !item.RequiresCitation {
		if item.CitationKind != "none" || len(item.ExpectedKnowledgeIDs) > 0 || len(item.ExpectedReportRefs) > 0 {
			*issues = append(*issues, prefix+": citation fields must be empty/none when requires_citation=false")
		}
		return
	}
	switch item.CitationKind {
	case "knowledge":
		if len(item.ExpectedKnowledgeIDs) == 0 || len(item.ExpectedReportRefs) > 0 {
			*issues = append(*issues, prefix+": knowledge citation requires knowledge IDs only")
		}
		if !containsAny(item.RequiredTools, "search_knowledge", "recommend_knowledge") && len(item.KnowledgeContext) == 0 {
			*issues = append(*issues, prefix+": knowledge citation has no knowledge source")
		}
	case "report":
		if len(item.ExpectedReportRefs) == 0 || len(item.ExpectedKnowledgeIDs) > 0 {
			*issues = append(*issues, prefix+": report citation requires report refs only")
		}
		if !containsAny(item.RequiredTools, "get_report_materials") {
			*issues = append(*issues, prefix+": report citation requires get_report_materials")
		}
	default:
		*issues = append(*issues, prefix+": citation_kind must be knowledge or report")
	}
}

func validateEvidenceFixtures(issues *validationIssues, prefix string, item *Case, fixtures BusinessFixtures) {
	if !item.RequiresCitation {
		return
	}
	switch item.CitationKind {
	case "knowledge":
		available := make(map[int64]struct{})
		for _, source := range item.KnowledgeContext {
			if source.ID > 0 {
				available[source.ID] = struct{}{}
			}
		}
		for _, toolName := range []string{"search_knowledge", "recommend_knowledge"} {
			data, ok := caseFixtureData(item, fixtures, toolName)
			if !ok {
				continue
			}
			var values []struct {
				ID     int64 `json:"id"`
				ItemID int64 `json:"item_id"`
			}
			if err := json.Unmarshal(data, &values); err != nil {
				*issues = append(*issues, fmt.Sprintf("%s: cannot inspect %s citation fixture: %v", prefix, toolName, err))
				continue
			}
			for _, value := range values {
				if value.ID > 0 {
					available[value.ID] = struct{}{}
				}
				if value.ItemID > 0 {
					available[value.ItemID] = struct{}{}
				}
			}
		}
		for _, id := range item.ExpectedKnowledgeIDs {
			if id <= 0 {
				*issues = append(*issues, fmt.Sprintf("%s: expected knowledge ID %d is invalid", prefix, id))
				continue
			}
			if _, ok := available[id]; !ok {
				*issues = append(*issues, fmt.Sprintf("%s: expected knowledge ID %d is absent from context/fixtures", prefix, id))
			}
		}
	case "report":
		data, ok := caseFixtureData(item, fixtures, "get_report_materials")
		if !ok {
			return
		}
		var reportFixture reportMaterialsFixture
		if err := json.Unmarshal(data, &reportFixture); err != nil {
			*issues = append(*issues, fmt.Sprintf("%s: cannot inspect report citation fixture: %v", prefix, err))
			return
		}
		if reportFixture.PeriodID <= 0 {
			*issues = append(*issues, prefix+": report fixture period_id must be positive")
		}
		reachable := make(map[string]struct{})
		for status, raw := range reportFixture.Reports {
			if status != "submitted" && status != "reviewed" {
				*issues = append(*issues, fmt.Sprintf("%s: report fixture contains unsupported status bucket %q", prefix, status))
			}
			var reports []struct {
				ID             int64 `json:"id"`
				PeriodID       int64 `json:"period_id"`
				CurrentVersion int   `json:"current_version"`
			}
			if err := json.Unmarshal(raw, &reports); err != nil {
				*issues = append(*issues, fmt.Sprintf("%s: cannot inspect report fixture bucket %s: %v", prefix, status, err))
				continue
			}
			for _, report := range reports {
				key := fmt.Sprintf("%d:%d", report.ID, report.CurrentVersion)
				if report.ID <= 0 || report.CurrentVersion <= 0 || report.PeriodID != reportFixture.PeriodID {
					*issues = append(*issues, fmt.Sprintf("%s: report fixture entry %s has invalid identity/period", prefix, key))
					continue
				}
				versionData, exists := reportFixture.Versions[key]
				if !exists {
					*issues = append(*issues, fmt.Sprintf("%s: report fixture entry %s has no reachable current version", prefix, key))
					continue
				}
				var version struct {
					ReportID int64 `json:"report_id"`
					Version  int   `json:"version"`
				}
				if err := json.Unmarshal(versionData, &version); err != nil || version.ReportID != report.ID || version.Version != report.CurrentVersion {
					*issues = append(*issues, fmt.Sprintf("%s: report fixture version %s does not match its report", prefix, key))
					continue
				}
				reachable[key] = struct{}{}
			}
		}
		for _, reference := range item.ExpectedReportRefs {
			match := reportRefPattern.FindStringSubmatch(reference)
			if match == nil {
				*issues = append(*issues, fmt.Sprintf("%s: malformed report reference %q", prefix, reference))
				continue
			}
			key := match[1] + ":" + match[2]
			if _, ok := reachable[key]; !ok {
				*issues = append(*issues, fmt.Sprintf("%s: report reference %s is not reachable from fixture reports", prefix, reference))
			}
		}
	}
}

func validateIdentityFixtures(issues *validationIssues, prefix string, item *Case, fixtures BusinessFixtures) {
	required := stringSet(item.RequiredTools)
	expectedUserID := evaluationUserID(item.Role)
	for _, toolName := range []string{"get_my_work_hours", "clock_in", "clock_out", "draft_announcement"} {
		if _, ok := required[toolName]; !ok {
			continue
		}
		data, ok := caseFixtureData(item, fixtures, toolName)
		if !ok {
			continue
		}
		var identity struct {
			UserID    int64 `json:"user_id"`
			CreatedBy int64 `json:"created_by"`
		}
		if err := json.Unmarshal(data, &identity); err != nil {
			*issues = append(*issues, fmt.Sprintf("%s: cannot inspect identity fixture %s: %v", prefix, toolName, err))
			continue
		}
		actual := identity.UserID
		field := "user_id"
		if toolName == "draft_announcement" {
			actual = identity.CreatedBy
			field = "created_by"
		}
		if actual != expectedUserID {
			*issues = append(*issues, fmt.Sprintf("%s: fixture %s %s=%d, want evaluation user %d", prefix, toolName, field, actual, expectedUserID))
		}
	}

	if _, ok := required["get_report_materials"]; !ok {
		return
	}
	mineExpected := false
	for _, projection := range item.ToolArguments["get_report_materials"] {
		scope, _ := effectiveArgumentProjection("get_report_materials", projection)["scope"].(string)
		if scope == "mine" {
			mineExpected = true
		}
	}
	data, ok := caseFixtureData(item, fixtures, "get_report_materials")
	if !ok {
		return
	}
	var fixture reportMaterialsFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		*issues = append(*issues, fmt.Sprintf("%s: cannot inspect mine report fixture: %v", prefix, err))
		return
	}
	if fixture.PeriodID <= 0 {
		*issues = append(*issues, prefix+": report fixture period_id must be positive")
	}
	if !mineExpected {
		return
	}
	for status, raw := range fixture.Reports {
		var reports []struct {
			ID       int64 `json:"id"`
			AuthorID int64 `json:"author_id"`
		}
		if err := json.Unmarshal(raw, &reports); err != nil {
			*issues = append(*issues, fmt.Sprintf("%s: cannot inspect mine report bucket %s: %v", prefix, status, err))
			continue
		}
		for _, report := range reports {
			if report.AuthorID != expectedUserID {
				*issues = append(*issues, fmt.Sprintf("%s: mine report %d author_id=%d, want evaluation user %d", prefix, report.ID, report.AuthorID, expectedUserID))
			}
		}
	}
}

func validatePrivacyCanaries(issues *validationIssues, prefix string, item *Case, fixtures BusinessFixtures) {
	if len(item.PrivacyCanaries) == 0 {
		return
	}
	var source strings.Builder
	source.WriteString(item.Prompt)
	source.WriteString(item.RollingSummary)
	for _, message := range item.History {
		source.WriteString(message.Content)
	}
	for _, knowledge := range item.KnowledgeContext {
		data, _ := json.Marshal(knowledge)
		source.Write(data)
	}
	for toolName := range item.Fixture {
		if data, ok := caseFixtureData(item, fixtures, toolName); ok {
			source.Write(data)
		}
	}
	combined := source.String()
	for _, canary := range item.PrivacyCanaries {
		if canary != "" && !strings.Contains(combined, canary) {
			*issues = append(*issues, fmt.Sprintf("%s: privacy canary %q is not seeded in case input/fixture", prefix, canary))
		}
	}
}

func caseFixtureData(item *Case, fixtures BusinessFixtures, toolName string) (json.RawMessage, bool) {
	variants, ok := fixtures.Tools[toolName]
	if !ok {
		return nil, false
	}
	variant := item.Fixture[toolName]
	if variant == "" {
		variant = "default"
	}
	data, ok := variants[variant]
	return data, ok
}

func validateToolList(issues *validationIssues, prefix, field string, values []string, known map[string]struct{}) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, ok := known[value]; !ok {
			*issues = append(*issues, fmt.Sprintf("%s: %s contains unknown tool %q", prefix, field, value))
		}
		if _, duplicate := result[value]; duplicate {
			*issues = append(*issues, fmt.Sprintf("%s: %s contains duplicate tool %q", prefix, field, value))
		}
		result[value] = struct{}{}
	}
	return result
}

func idMatchesRole(id, role string) bool {
	if id == "" {
		return false
	}
	prefixes := map[string]byte{"member": 'M', "captain": 'C', "super_admin": 'S'}
	return id[0] == prefixes[role]
}

func containsWriteTool(values []string) bool {
	for _, value := range values {
		if _, ok := writeToolNames[value]; ok {
			return true
		}
	}
	return false
}

func containsAny(values []string, candidates ...string) bool {
	set := stringSet(values)
	for _, candidate := range candidates {
		if _, ok := set[candidate]; ok {
			return true
		}
	}
	return false
}

func hasEmptyOrDuplicate(values []string) bool {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			return true
		}
		if _, ok := seen[value]; ok {
			return true
		}
		seen[value] = struct{}{}
	}
	return false
}

func stringSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		result[value] = struct{}{}
	}
	return result
}

func sameStringSet(left, right []string) bool {
	if len(left) != len(right) || hasEmptyOrDuplicate(left) || hasEmptyOrDuplicate(right) {
		return false
	}
	leftSet := stringSet(left)
	for _, value := range right {
		if _, ok := leftSet[value]; !ok {
			return false
		}
	}
	return true
}

func mapKeys[V any](values map[string]V) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	return result
}

func allowedToolsForRole(role string, writeEnabled bool) []string {
	result := make([]string, 0, len(allToolNames))
	for _, toolName := range allToolNames {
		if role == "member" {
			if _, forbidden := memberForbiddenTools[toolName]; forbidden {
				continue
			}
		}
		if _, write := writeToolNames[toolName]; write && !writeEnabled {
			continue
		}
		result = append(result, toolName)
	}
	return result
}

func subtractStrings(all, removed []string) []string {
	removedSet := stringSet(removed)
	result := make([]string, 0, len(all))
	for _, value := range all {
		if _, ok := removedSet[value]; !ok {
			result = append(result, value)
		}
	}
	return result
}
