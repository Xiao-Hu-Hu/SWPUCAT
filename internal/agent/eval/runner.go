package agenteval

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"SWPUCAT/internal/agent"
	"SWPUCAT/internal/agent/business"
	"SWPUCAT/internal/infrastructure/config"
)

// Coverage is a deterministic summary of the selected declarative cases. Tool
// counts describe required calls, rather than merely allowed calls, so a suite
// cannot claim coverage for a tool it never expects the model to use.
type Coverage struct {
	Cases    int            `json:"cases"`
	Roles    map[string]int `json:"roles"`
	Tools    map[string]int `json:"tools"`
	Outcomes map[string]int `json:"outcomes"`
	Tags     map[string]int `json:"tags"`
}

// CaseRun contains one scored live execution. Observation is included so a
// failing model run is reproducible without exposing raw prompts or tool
// results; tool arguments have already passed through production telemetry's
// privacy-safe projection.
type CaseRun struct {
	CaseID      string      `json:"case_id"`
	Role        string      `json:"role"`
	Iteration   int         `json:"iteration"`
	Observation Observation `json:"observation"`
	Score       Score       `json:"score"`
}

// Report is shared by offline validation and opt-in live model evaluation.
// Runs is empty in validate mode because that mode never loads configuration,
// starts a server, or makes a network request.
type Report struct {
	Mode             string    `json:"mode"`
	Suite            string    `json:"suite"`
	SuiteID          string    `json:"suite_id"`
	Role             string    `json:"role"`
	Repeat           int       `json:"repeat"`
	SuiteCases       int       `json:"suite_cases"`
	SelectedCases    int       `json:"selected_cases"`
	Coverage         Coverage  `json:"coverage"`
	Runs             []CaseRun `json:"runs,omitempty"`
	PassedRuns       int       `json:"passed_runs"`
	FailedRuns       int       `json:"failed_runs"`
	Success          bool      `json:"success"`
	ValidationNotice string    `json:"validation_notice,omitempty"`
}

// Validate performs the default, completely offline validation. LoadEmbedded
// applies the strict schema and coverage invariants before a report is built.
func Validate(name string) (*Report, error) {
	return ValidateRole(name, "all")
}

// ValidateRole is the role-filtered form used by the CLI. It remains fully
// offline: in particular, it must not call config.Load or create an HTTP
// fixture server.
func ValidateRole(name, role string) (*Report, error) {
	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}
	suite, err := LoadEmbedded(name)
	if err != nil {
		return nil, err
	}
	cases := selectCases(suite.Cases, role)
	if len(cases) == 0 {
		return nil, fmt.Errorf("suite %q has no cases for role %q", name, role)
	}
	return &Report{
		Mode:             "validate",
		Suite:            name,
		SuiteID:          suite.Manifest.SuiteID,
		Role:             role,
		Repeat:           1,
		SuiteCases:       len(suite.Cases),
		SelectedCases:    len(cases),
		Coverage:         coverageFor(cases),
		Success:          true,
		ValidationNotice: "仅验证数据集与确定性评分器，不代表模型质量通过",
	}, nil
}

// RunLive runs the selected cases against the configured LLM and a local,
// deterministic business-API fixture. Calling this function is deliberately
// explicit; the ordinary validation path cannot reach it by accident.
func RunLive(ctx context.Context, name, role string, repeat int, base config.AgentConfig) (*Report, error) {
	if ctx == nil {
		return nil, fmt.Errorf("live evaluation context is required")
	}
	role, err := normalizeRole(role)
	if err != nil {
		return nil, err
	}
	if repeat < 1 {
		return nil, fmt.Errorf("repeat must be at least 1")
	}
	suite, err := LoadEmbedded(name)
	if err != nil {
		return nil, err
	}
	return runLoadedLive(ctx, name, role, repeat, base, suite)
}

func runLoadedLive(ctx context.Context, name, role string, repeat int, base config.AgentConfig, suite *Suite) (*Report, error) {
	if suite == nil {
		return nil, fmt.Errorf("evaluation suite is required")
	}
	cases := selectCases(suite.Cases, role)
	if len(cases) == 0 {
		return nil, fmt.Errorf("suite %q has no cases for role %q", name, role)
	}

	fixture, err := NewFixtureServer(suite)
	if err != nil {
		return nil, fmt.Errorf("start evaluation fixture server: %w", err)
	}
	defer fixture.Close()

	readEngine, err := agent.NewEngine(ctx, liveConfig(base, fixture.URL(), false))
	if err != nil {
		return nil, fmt.Errorf("initialize read-only evaluation engine: %w", err)
	}
	writeEngine, err := agent.NewEngine(ctx, liveConfig(base, fixture.URL(), true))
	if err != nil {
		return nil, fmt.Errorf("initialize write-enabled evaluation engine: %w", err)
	}

	report := &Report{
		Mode:          "live",
		Suite:         name,
		SuiteID:       suite.Manifest.SuiteID,
		Role:          role,
		Repeat:        repeat,
		SuiteCases:    len(suite.Cases),
		SelectedCases: len(cases),
		Coverage:      coverageFor(cases),
		Runs:          make([]CaseRun, 0, len(cases)*repeat),
	}
	for iteration := 1; iteration <= repeat; iteration++ {
		for _, item := range cases {
			if err := ctx.Err(); err != nil {
				return report, err
			}
			if err := fixture.BeginCase(item); err != nil {
				return report, fmt.Errorf("prepare fixture for case %s: %w", item.ID, err)
			}

			engine := readEngine
			if item.WriteEnabled {
				engine = writeEngine
			}
			response, chatErr := engine.Chat(ctx, chatRequest(item, iteration))
			observation := observationFrom(response, fixture.Ledger(item.ID), chatErr)
			score := ScoreCase(item, observation)
			report.Runs = append(report.Runs, CaseRun{
				CaseID:      item.ID,
				Role:        item.Role,
				Iteration:   iteration,
				Observation: observation,
				Score:       score,
			})
			if score.Passed {
				report.PassedRuns++
			} else {
				report.FailedRuns++
			}
		}
	}
	report.Success = report.FailedRuns == 0
	return report, nil
}

func liveConfig(base config.AgentConfig, serverURL string, writeEnabled bool) config.AgentConfig {
	// Only the LLM settings are inherited from process configuration. All
	// operational and business-server settings are evaluation-owned so an
	// accidental CSA_AGENT_* override cannot point a live evaluation at the
	// production application.
	return config.AgentConfig{
		Enabled:           true,
		WriteEnabled:      writeEnabled,
		Port:              8090,
		BaseURL:           "http://127.0.0.1:8090",
		ServerURL:         serverURL,
		InternalSecret:    FixtureInternalSecret,
		LLMProvider:       base.LLMProvider,
		LLMModel:          base.LLMModel,
		LLMAPIKey:         base.LLMAPIKey,
		LLMBaseURL:        base.LLMBaseURL,
		LLMTimeout:        base.LLMTimeout,
		MaxIterations:     4,
		MaxToolCalls:      8,
		WorkerConcurrency: 1,
	}
}

func chatRequest(item Case, iteration int) agent.ChatRequest {
	history := make([]agent.HistoryMessage, 0, len(item.History))
	for _, message := range item.History {
		history = append(history, agent.HistoryMessage{Role: message.Role, Content: message.Content})
	}
	knowledge := make([]business.KnowledgeSource, 0, len(item.KnowledgeContext))
	for _, source := range item.KnowledgeContext {
		knowledge = append(knowledge, business.KnowledgeSource{
			ID:           source.ID,
			Type:         source.Type,
			Name:         source.Name,
			Description:  source.Description,
			URL:          source.URL,
			DownloadURL:  source.DownloadURL,
			CategoryName: source.CategoryName,
			SourceKind:   source.SourceKind,
			Excerpt:      source.Excerpt,
			Score:        source.Score,
			MatchMode:    source.MatchMode,
			CreatedAt:    source.CreatedAt,
		})
	}
	runID := fmt.Sprintf("eval-%s-%d", strings.ToLower(item.ID), iteration)
	return agent.ChatRequest{
		Message:          item.Prompt,
		SessionID:        runID,
		UserID:           evaluationUserID(item.Role),
		Role:             item.Role,
		HistoryProvided:  true,
		History:          history,
		RollingSummary:   item.RollingSummary,
		KnowledgeContext: knowledge,
		RequestID:        runID,
		Authorization:    "Bearer eval/" + item.ID,
	}
}

func observationFrom(response agent.ChatResponse, ledger CaseLedger, chatErr error) Observation {
	toolCalls := make([]ObservedToolCall, 0, len(response.Telemetry.ToolCalls))
	for _, call := range response.Telemetry.ToolCalls {
		arguments, err := DecodeArgumentProjection(call.Arguments.JSON)
		if err != nil {
			// A malformed telemetry projection is itself an execution failure;
			// retain the call name/status while making the failure visible to
			// the deterministic scorer.
			arguments = map[string]any{}
			if chatErr == nil {
				chatErr = fmt.Errorf("decode %s tool arguments: %w", call.Name, err)
			}
		}
		toolCalls = append(toolCalls, ObservedToolCall{
			Name:      call.Name,
			Status:    call.Status,
			Arguments: arguments,
		})
	}
	sourceIDs := make([]int64, 0, len(response.Sources))
	for _, source := range response.Sources {
		sourceIDs = append(sourceIDs, source.ID)
	}
	observation := Observation{
		Answer:            response.Answer,
		SourceIDs:         sourceIDs,
		FetchedReportRefs: fetchedReportReferences(ledger),
		ToolCalls:         toolCalls,
		WriteAttempts:     ledger.WriteAttempts,
		WriteSucceeded:    ledger.WriteSucceeded,
	}
	if chatErr != nil {
		observation.ExecutionError = chatErr.Error()
	}
	return observation
}

func fetchedReportReferences(ledger CaseLedger) []string {
	result := make([]string, 0)
	for _, request := range ledger.Requests {
		if !request.Succeeded {
			continue
		}
		route, ok := fixtureRouteForPath(request.Path)
		if !ok || route.tool != "get_report_materials" || route.versionKey == "" {
			continue
		}
		reportID, version, ok := strings.Cut(route.versionKey, ":")
		if !ok {
			continue
		}
		result = append(result, fmt.Sprintf("[报告#%s-v%s]", reportID, version))
	}
	return uniqueSortedStrings(result)
}

func normalizeRole(role string) (string, error) {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "all", "member", "captain", "super_admin":
		return role, nil
	default:
		return "", fmt.Errorf("role must be one of all, member, captain, or super_admin")
	}
}

func selectCases(cases []Case, role string) []Case {
	if role == "all" {
		return append([]Case(nil), cases...)
	}
	selected := make([]Case, 0)
	for _, item := range cases {
		if item.Role == role {
			selected = append(selected, item)
		}
	}
	return selected
}

func coverageFor(cases []Case) Coverage {
	coverage := Coverage{
		Cases:    len(cases),
		Roles:    make(map[string]int),
		Tools:    make(map[string]int),
		Outcomes: make(map[string]int),
		Tags:     make(map[string]int),
	}
	for _, item := range cases {
		coverage.Roles[item.Role]++
		coverage.Outcomes[item.ExpectedOutcome]++
		for _, toolName := range item.RequiredTools {
			coverage.Tools[toolName]++
		}
		for _, tag := range item.Tags {
			coverage.Tags[tag]++
		}
	}
	return coverage
}

// SortedCoverageKeys is used by the CLI and is exported so other report
// renderers can preserve deterministic output.
func SortedCoverageKeys(values map[string]int) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func evaluationUserID(role string) int64 {
	switch role {
	case "member":
		return 7
	case "captain":
		return 9
	case "super_admin":
		return 1
	default:
		return 0
	}
}
