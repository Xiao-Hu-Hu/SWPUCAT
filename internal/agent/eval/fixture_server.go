package agenteval

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
)

// FixtureInternalSecret is the non-production shared secret used by the live
// evaluation runner. Authentication of an evaluation case is deliberately
// based on its Bearer token; this constant merely lets the runner configure the
// production business client without introducing a real secret.
const FixtureInternalSecret = "agent-eval-internal-secret-00000000"

const maxFixtureRequestBody = 1 << 20

// LedgerRequest is a privacy-safe record of one request made to the synthetic
// business API. Body is retained because evaluation writes contain only the
// suite's synthetic data and it is useful when diagnosing a failed case.
type LedgerRequest struct {
	Sequence       int             `json:"sequence"`
	Method         string          `json:"method"`
	Path           string          `json:"path"`
	Query          url.Values      `json:"query,omitempty"`
	Tool           string          `json:"tool,omitempty"`
	FixtureVariant string          `json:"fixture_variant,omitempty"`
	Body           json.RawMessage `json:"body,omitempty"`
	IdempotencyKey string          `json:"idempotency_key,omitempty"`
	StatusCode     int             `json:"status_code"`
	Write          bool            `json:"write"`
	Succeeded      bool            `json:"succeeded"`
}

// CaseLedger summarizes the HTTP-side effects observed for one evaluation
// case. ToolCalls includes rejected calls to a known route, making an attempted
// capability escape visible even when the fixture server correctly blocks it.
type CaseLedger struct {
	CaseID          string          `json:"case_id"`
	Requests        []LedgerRequest `json:"requests"`
	ToolCalls       map[string]int  `json:"tool_calls"`
	WriteAttempts   int             `json:"write_attempts"`
	WriteSucceeded  int             `json:"write_succeeded"`
	IdempotencyKeys []string        `json:"idempotency_keys,omitempty"`
}

type boundFixtureCase struct {
	id           string
	role         string
	userID       int64
	writeEnabled bool
	allowed      map[string]struct{}
	forbidden    map[string]struct{}
	variants     map[string]string
	ledger       CaseLedger
}

// FixtureServer serves deterministic business API responses for a Suite. A
// case is inaccessible until BeginCase binds its Bearer token.
type FixtureServer struct {
	mu        sync.Mutex
	server    *httptest.Server
	fixtures  BusinessFixtures
	knownCase map[string]struct{}
	bound     map[string]*boundFixtureCase
	closeOnce sync.Once
}

// NewFixtureServer starts a loopback-only synthetic business API.
func NewFixtureServer(suite *Suite) (*FixtureServer, error) {
	if suite == nil {
		return nil, errors.New("evaluation suite is required")
	}
	if suite.Fixtures.Version != 1 {
		return nil, fmt.Errorf("fixture version=%d, want 1", suite.Fixtures.Version)
	}
	if len(suite.Fixtures.Tools) == 0 {
		return nil, errors.New("business fixtures are empty")
	}

	fixtures := BusinessFixtures{
		Version:      suite.Fixtures.Version,
		Tools:        make(map[string]map[string]json.RawMessage, len(suite.Fixtures.Tools)),
		RequestRules: make(map[string]map[string]FixtureRequestRule, len(suite.Fixtures.RequestRules)),
	}
	for toolName, variants := range suite.Fixtures.Tools {
		fixtures.Tools[toolName] = make(map[string]json.RawMessage, len(variants))
		for variant, data := range variants {
			if !json.Valid(data) {
				return nil, fmt.Errorf("fixture %s/%s is invalid JSON", toolName, variant)
			}
			fixtures.Tools[toolName][variant] = append(json.RawMessage(nil), data...)
		}
	}
	for toolName, variants := range suite.Fixtures.RequestRules {
		fixtures.RequestRules[toolName] = make(map[string]FixtureRequestRule, len(variants))
		for variant, rule := range variants {
			rule.QueryTerms = append([]string(nil), rule.QueryTerms...)
			fixtures.RequestRules[toolName][variant] = rule
		}
	}

	result := &FixtureServer{
		fixtures:  fixtures,
		knownCase: make(map[string]struct{}, len(suite.Cases)),
		bound:     make(map[string]*boundFixtureCase),
	}
	for _, item := range suite.Cases {
		if _, duplicate := result.knownCase[item.ID]; duplicate {
			return nil, fmt.Errorf("duplicate evaluation case %q", item.ID)
		}
		result.knownCase[item.ID] = struct{}{}
	}
	result.server = httptest.NewServer(http.HandlerFunc(result.serveHTTP))
	return result, nil
}

func (s *FixtureServer) URL() string {
	if s == nil || s.server == nil {
		return ""
	}
	return s.server.URL
}

func (s *FixtureServer) Close() {
	if s == nil {
		return
	}
	s.closeOnce.Do(func() {
		if s.server != nil {
			s.server.Close()
		}
	})
}

// BeginCase binds Bearer eval/<case-id> and resets that case's ledger. It is
// intentionally safe to call again for repeated model runs of the same case.
func (s *FixtureServer) BeginCase(item Case) error {
	if s == nil || s.server == nil {
		return errors.New("fixture server is not initialized")
	}
	if !caseIDPattern.MatchString(item.ID) {
		return fmt.Errorf("invalid evaluation case id %q", item.ID)
	}
	if _, ok := s.knownCase[item.ID]; !ok {
		return fmt.Errorf("evaluation case %q is not part of the suite", item.ID)
	}
	allowed := make(map[string]struct{}, len(item.AllowedTools))
	for _, toolName := range item.AllowedTools {
		if !knownFixtureTool(toolName) {
			return fmt.Errorf("case %s allows unknown tool %q", item.ID, toolName)
		}
		allowed[toolName] = struct{}{}
	}
	forbidden := make(map[string]struct{}, len(item.ForbiddenTools))
	for _, toolName := range item.ForbiddenTools {
		if !knownFixtureTool(toolName) {
			return fmt.Errorf("case %s forbids unknown tool %q", item.ID, toolName)
		}
		forbidden[toolName] = struct{}{}
	}
	variants := make(map[string]string, len(item.Fixture))
	for toolName, variant := range item.Fixture {
		if !knownFixtureTool(toolName) {
			return fmt.Errorf("case %s selects fixture for unknown tool %q", item.ID, toolName)
		}
		if _, ok := s.fixtures.Tools[toolName][variant]; !ok {
			return fmt.Errorf("case %s selects missing fixture %s/%s", item.ID, toolName, variant)
		}
		variants[toolName] = variant
	}
	for toolName := range allowed {
		variant := variants[toolName]
		if variant == "" {
			variant = "default"
		}
		if _, ok := s.fixtures.Tools[toolName][variant]; !ok {
			return fmt.Errorf("case %s has no fixture %s/%s", item.ID, toolName, variant)
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.bound[item.ID] = &boundFixtureCase{
		id:           item.ID,
		role:         item.Role,
		userID:       evaluationUserID(item.Role),
		writeEnabled: item.WriteEnabled,
		allowed:      allowed,
		forbidden:    forbidden,
		variants:     variants,
		ledger: CaseLedger{
			CaseID:    item.ID,
			Requests:  make([]LedgerRequest, 0),
			ToolCalls: make(map[string]int),
		},
	}
	return nil
}

// Ledger returns a detached snapshot safe for callers to inspect while other
// evaluation cases continue running.
func (s *FixtureServer) Ledger(caseID string) CaseLedger {
	if s == nil {
		return CaseLedger{CaseID: caseID, ToolCalls: map[string]int{}}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.bound[caseID]
	if !ok {
		return CaseLedger{CaseID: caseID, ToolCalls: map[string]int{}}
	}
	return cloneCaseLedger(item.ledger)
}

// Observation is an alias kept for consumers that prefer an observation-style
// API. Ledger remains the canonical name used by the live runner.
func (s *FixtureServer) Observation(caseID string) CaseLedger {
	return s.Ledger(caseID)
}

type fixtureRoute struct {
	tool       string
	method     string
	write      bool
	reportList bool
	versionKey string
}

var staticFixtureRoutes = map[string]fixtureRoute{
	"/api/checkin/status":            {tool: "get_checkin_status", method: http.MethodGet},
	"/api/checkin/me/work-hours":     {tool: "get_my_work_hours", method: http.MethodGet},
	"/api/checkin/records":           {tool: "get_my_checkin_history", method: http.MethodGet},
	"/api/checkin/requirements":      {tool: "get_checkin_requirements", method: http.MethodGet},
	"/api/checkin/online-summary":    {tool: "get_online_summary", method: http.MethodGet},
	"/api/checkin/clock-in":          {tool: "clock_in", method: http.MethodPost, write: true},
	"/api/checkin/clock-out":         {tool: "clock_out", method: http.MethodPost, write: true},
	"/api/knowledge/search":          {tool: "search_knowledge", method: http.MethodGet},
	"/api/knowledge/recommendations": {tool: "recommend_knowledge", method: http.MethodGet},
	"/api/knowledge/items/pending":   {tool: "list_pending_knowledge", method: http.MethodGet},
	"/api/checkin/stats":             {tool: "get_checkin_stats", method: http.MethodGet},
	"/api/analysis/team":             {tool: "query_team_members", method: http.MethodGet},
	"/api/report-periods":            {tool: "list_report_periods", method: http.MethodGet},
	"/api/reports":                   {tool: "get_report_materials", method: http.MethodGet, reportList: true},
	"/api/announcement-drafts":       {tool: "draft_announcement", method: http.MethodPost, write: true},
}

func (s *FixtureServer) serveHTTP(w http.ResponseWriter, r *http.Request) {
	_, item := s.authenticate(r.Header.Get("Authorization"))
	if item == nil {
		writeFixtureError(w, http.StatusUnauthorized, "unknown or unbound evaluation token")
		return
	}

	route, found := fixtureRouteForPath(r.URL.Path)
	body, bodyErr := readFixtureBody(r.Body)
	entry := LedgerRequest{
		Method:         r.Method,
		Path:           r.URL.Path,
		Query:          cloneValues(r.URL.Query()),
		Body:           body,
		IdempotencyKey: r.Header.Get("Idempotency-Key"),
	}
	if found {
		entry.Tool = route.tool
		entry.Write = route.write && r.Method == route.method
		entry.FixtureVariant = item.fixtureVariant(route.tool)
	}

	status := http.StatusOK
	var data json.RawMessage
	var failure string
	switch {
	case bodyErr != nil:
		status, failure = http.StatusRequestEntityTooLarge, bodyErr.Error()
	case !found:
		status, failure = http.StatusNotFound, "unknown business API route"
	case r.Method != route.method:
		status, failure = http.StatusMethodNotAllowed, "method is not allowed for this route"
	case !item.allows(route):
		status, failure = http.StatusForbidden, "tool route is not allowed for this evaluation case"
	default:
		var err error
		if err = s.validateRequest(item, route, r.URL.Query(), body, entry.IdempotencyKey); err == nil {
			data, err = s.responseData(item, route, r.URL.Query())
		}
		if err != nil {
			var requestErr *fixtureRequestError
			if errors.As(err, &requestErr) {
				status, failure = requestErr.status, requestErr.message
			} else {
				status, failure = http.StatusInternalServerError, err.Error()
			}
		}
	}

	entry.StatusCode = status
	entry.Succeeded = status >= http.StatusOK && status < http.StatusMultipleChoices
	s.record(item, entry)
	if failure != "" {
		writeFixtureError(w, status, failure)
		return
	}
	writeFixtureSuccess(w, data)
}

func (s *FixtureServer) authenticate(header string) (string, *boundFixtureCase) {
	const prefix = "Bearer eval/"
	if !strings.HasPrefix(header, prefix) || len(header) == len(prefix) || strings.ContainsAny(header[len(prefix):], " /\t\r\n") {
		return "", nil
	}
	caseID := header[len(prefix):]
	s.mu.Lock()
	defer s.mu.Unlock()
	return caseID, s.bound[caseID]
}

func (item *boundFixtureCase) fixtureVariant(toolName string) string {
	if variant := item.variants[toolName]; variant != "" {
		return variant
	}
	return "default"
}

func (item *boundFixtureCase) allows(route fixtureRoute) bool {
	if _, ok := item.allowed[route.tool]; !ok {
		return false
	}
	if _, forbidden := item.forbidden[route.tool]; forbidden {
		return false
	}
	if route.write && !item.writeEnabled {
		return false
	}
	switch route.tool {
	case "query_team_members", "get_checkin_stats", "list_pending_knowledge", "draft_announcement":
		return item.role == "captain" || item.role == "super_admin"
	default:
		return true
	}
}

func (s *FixtureServer) responseData(item *boundFixtureCase, route fixtureRoute, query url.Values) (json.RawMessage, error) {
	variant := item.fixtureVariant(route.tool)
	data, ok := s.fixtures.Tools[route.tool][variant]
	if !ok {
		return nil, fmt.Errorf("fixture %s/%s is missing", route.tool, variant)
	}
	if route.reportList {
		return reportListData(data, query)
	}
	if route.versionKey != "" {
		return reportVersionData(data, route.versionKey)
	}
	if err := validateIdentityResponse(item, route, data); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), data...), nil
}

func (s *FixtureServer) validateRequest(item *boundFixtureCase, route fixtureRoute, query url.Values, body json.RawMessage, idempotencyKey string) error {
	data := s.fixtures.Tools[route.tool][item.fixtureVariant(route.tool)]
	if route.method == http.MethodGet && len(body) != 0 {
		return newFixtureRequestError(http.StatusBadRequest, "GET requests must not include a body")
	}
	switch route.tool {
	case "get_checkin_status", "get_my_work_hours", "get_checkin_requirements", "list_report_periods", "list_pending_knowledge", "get_online_summary":
		return rejectUnexpectedQuery(query)
	case "get_my_checkin_history":
		if _, err := positiveQueryInt(query, "limit", 20); err != nil {
			return err
		}
		return rejectUnexpectedQuery(query, "limit")
	case "search_knowledge", "recommend_knowledge":
		rule, ok := s.fixtures.RequestRules[route.tool][item.fixtureVariant(route.tool)]
		if !ok {
			return fmt.Errorf("fixture %s/%s has no request rule", route.tool, item.fixtureVariant(route.tool))
		}
		return validateKnowledgeQuery(query, rule)
	case "query_team_members":
		return validateTeamQuery(data, query)
	case "get_checkin_stats":
		period := query.Get("period")
		if period != "today" && period != "week" && period != "month" {
			return newFixtureRequestError(http.StatusBadRequest, "period must be today, week, or month")
		}
		return rejectUnexpectedQuery(query, "period")
	case "draft_announcement":
		if err := rejectUnexpectedQuery(query); err != nil {
			return err
		}
		return validateAnnouncementDraft(data, body)
	case "clock_in", "clock_out":
		if len(body) != 0 {
			return newFixtureRequestError(http.StatusBadRequest, "clock requests must not include a body")
		}
		if err := rejectUnexpectedQuery(query); err != nil {
			return err
		}
		action := "clock-in"
		if route.tool == "clock_out" {
			action = "clock-out"
		}
		if !validEvaluationIdempotencyKey(item.id, action, idempotencyKey) {
			return newFixtureRequestError(http.StatusBadRequest, "idempotency key does not match the evaluation request")
		}
	case "get_report_materials":
		if route.versionKey != "" {
			return rejectUnexpectedQuery(query)
		}
	}
	return nil
}

func validEvaluationIdempotencyKey(caseID, action, value string) bool {
	prefix := "agent:eval-" + strings.ToLower(caseID) + "-"
	suffix := ":" + action
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) {
		return false
	}
	iteration := strings.TrimSuffix(strings.TrimPrefix(value, prefix), suffix)
	parsed, err := strconv.Atoi(iteration)
	return err == nil && parsed > 0
}

func validateKnowledgeQuery(query url.Values, rule FixtureRequestRule) error {
	values, ok := query["q"]
	if !ok || len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return newFixtureRequestError(http.StatusBadRequest, "q must be provided exactly once")
	}
	if err := rejectUnexpectedQuery(query, "q"); err != nil {
		return err
	}
	normalized := strings.ToLower(strings.TrimSpace(values[0]))
	matches := 0
	for _, term := range rule.QueryTerms {
		if strings.Contains(normalized, strings.ToLower(term)) {
			matches++
		}
	}
	if matches < rule.MinQueryTermMatches {
		return newFixtureRequestError(http.StatusBadRequest, "knowledge query does not match the selected fixture")
	}
	return nil
}

type teamQueryFixture struct {
	Period       string   `json:"period"`
	PeriodID     *int64   `json:"period_id"`
	MinHours     *float64 `json:"min_hours"`
	ReportStatus string   `json:"report_status"`
	Limit        int      `json:"limit"`
}

func validateTeamQuery(data json.RawMessage, query url.Values) error {
	var expected teamQueryFixture
	if err := json.Unmarshal(data, &expected); err != nil {
		return fmt.Errorf("decode team-query fixture: %w", err)
	}
	if expected.Period == "" {
		expected.Period = "week"
	}
	if expected.Limit == 0 {
		expected.Limit = 100
	}
	period := query.Get("period")
	if period == "" {
		period = "week"
	}
	limit, err := positiveQueryInt(query, "limit", 100)
	if err != nil {
		return err
	}
	if period != expected.Period || limit != expected.Limit || query.Get("report_status") != expected.ReportStatus {
		return newFixtureRequestError(http.StatusBadRequest, "team query does not match the selected fixture")
	}
	if err := matchOptionalInt64Query(query, "period_id", expected.PeriodID); err != nil {
		return err
	}
	if err := matchOptionalFloatQuery(query, "min_hours", expected.MinHours); err != nil {
		return err
	}
	return rejectUnexpectedQuery(query, "period", "period_id", "min_hours", "report_status", "limit")
}

type announcementDraftFixture struct {
	Title          string  `json:"title"`
	Content        string  `json:"content"`
	Pinned         bool    `json:"pinned"`
	SourceJSON     string  `json:"source_json"`
	RecipientIDs   []int64 `json:"recipient_ids"`
	EmailRequested bool    `json:"email_requested"`
}

type announcementDraftRequest struct {
	Title          string         `json:"title"`
	Content        string         `json:"content"`
	Pinned         bool           `json:"pinned"`
	Source         map[string]any `json:"source"`
	RecipientIDs   []int64        `json:"recipient_ids"`
	EmailRequested bool           `json:"email_requested"`
}

func validateAnnouncementDraft(data, body json.RawMessage) error {
	var expected announcementDraftFixture
	if err := json.Unmarshal(data, &expected); err != nil {
		return fmt.Errorf("decode announcement fixture: %w", err)
	}
	var request announcementDraftRequest
	if err := decodeStrictJSON(body, &request); err != nil {
		return newFixtureRequestError(http.StatusBadRequest, "invalid announcement draft request: "+err.Error())
	}
	var expectedSource map[string]any
	if err := json.Unmarshal([]byte(expected.SourceJSON), &expectedSource); err != nil {
		return fmt.Errorf("decode announcement source fixture: %w", err)
	}
	if expected.RecipientIDs == nil {
		expected.RecipientIDs = []int64{}
	}
	if request.RecipientIDs == nil {
		request.RecipientIDs = []int64{}
	}
	if request.Title != expected.Title || request.Content != expected.Content || request.Pinned != expected.Pinned ||
		request.EmailRequested != expected.EmailRequested || !reflect.DeepEqual(request.RecipientIDs, expected.RecipientIDs) ||
		!jsonValueEqual(request.Source, expectedSource) {
		return newFixtureRequestError(http.StatusBadRequest, "announcement draft request does not match the selected fixture")
	}
	return nil
}

func validateIdentityResponse(item *boundFixtureCase, route fixtureRoute, data json.RawMessage) error {
	var identity struct {
		UserID    int64 `json:"user_id"`
		CreatedBy int64 `json:"created_by"`
	}
	switch route.tool {
	case "get_my_work_hours", "clock_in", "clock_out":
		if err := json.Unmarshal(data, &identity); err != nil {
			return fmt.Errorf("decode identity-bound fixture: %w", err)
		}
		if identity.UserID != item.userID {
			return fmt.Errorf("fixture %s user_id=%d does not match evaluation user %d", route.tool, identity.UserID, item.userID)
		}
	case "draft_announcement":
		if err := json.Unmarshal(data, &identity); err != nil {
			return fmt.Errorf("decode announcement identity fixture: %w", err)
		}
		if identity.CreatedBy != item.userID {
			return fmt.Errorf("fixture %s created_by=%d does not match evaluation user %d", route.tool, identity.CreatedBy, item.userID)
		}
	}
	return nil
}

func (s *FixtureServer) record(item *boundFixtureCase, entry LedgerRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry.Sequence = len(item.ledger.Requests) + 1
	entry.Query = cloneValues(entry.Query)
	entry.Body = append(json.RawMessage(nil), entry.Body...)
	item.ledger.Requests = append(item.ledger.Requests, entry)
	if entry.Tool != "" {
		item.ledger.ToolCalls[entry.Tool]++
	}
	if entry.Write {
		item.ledger.WriteAttempts++
		if entry.Succeeded {
			item.ledger.WriteSucceeded++
		}
	}
	if entry.IdempotencyKey != "" {
		item.ledger.IdempotencyKeys = append(item.ledger.IdempotencyKeys, entry.IdempotencyKey)
	}
}

func fixtureRouteForPath(path string) (fixtureRoute, bool) {
	if route, ok := staticFixtureRoutes[path]; ok {
		return route, true
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) != 5 || parts[0] != "api" || parts[1] != "reports" || parts[3] != "versions" {
		return fixtureRoute{}, false
	}
	reportID, reportErr := strconv.ParseInt(parts[2], 10, 64)
	version, versionErr := strconv.Atoi(parts[4])
	if reportErr != nil || versionErr != nil || reportID <= 0 || version <= 0 {
		return fixtureRoute{}, false
	}
	return fixtureRoute{
		tool:       "get_report_materials",
		method:     http.MethodGet,
		versionKey: fmt.Sprintf("%d:%d", reportID, version),
	}, true
}

func knownFixtureTool(toolName string) bool {
	for _, candidate := range allToolNames {
		if toolName == candidate {
			return true
		}
	}
	return false
}

type reportMaterialsFixture struct {
	PeriodID int64                      `json:"period_id"`
	Reports  map[string]json.RawMessage `json:"reports"`
	Versions map[string]json.RawMessage `json:"versions"`
}

func reportListData(data json.RawMessage, query url.Values) (json.RawMessage, error) {
	var fixture reportMaterialsFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("decode report material fixture: %w", err)
	}
	if fixture.PeriodID <= 0 {
		return nil, fmt.Errorf("report material fixture period_id must be positive")
	}
	periodID, err := requiredPositiveQueryInt64(query, "period_id")
	if err != nil {
		return nil, err
	}
	if periodID != fixture.PeriodID {
		return nil, newFixtureRequestError(http.StatusBadRequest, "period_id does not match the selected report fixture")
	}
	statuses := []string{query.Get("status")}
	if statuses[0] == "" {
		statuses = []string{"submitted", "reviewed"}
	} else if statuses[0] != "submitted" && statuses[0] != "reviewed" {
		return nil, newFixtureRequestError(http.StatusBadRequest, "status must be submitted or reviewed")
	}
	reports := make([]json.RawMessage, 0)
	for _, status := range statuses {
		raw, ok := fixture.Reports[status]
		if !ok {
			continue
		}
		var batch []json.RawMessage
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, fmt.Errorf("decode report fixture status %s: %w", status, err)
		}
		reports = append(reports, batch...)
	}

	filtered := reports[:0]
	for _, report := range reports {
		var identity struct {
			PeriodID int64 `json:"period_id"`
		}
		if err := json.Unmarshal(report, &identity); err != nil {
			return nil, fmt.Errorf("decode report identity: %w", err)
		}
		if identity.PeriodID == periodID {
			filtered = append(filtered, report)
		}
	}
	reports = filtered

	page, err := positiveQueryInt(query, "page", 1)
	if err != nil {
		return nil, err
	}
	pageSize, err := positiveQueryInt(query, "page_size", 100)
	if err != nil {
		return nil, err
	}
	if page-1 > len(reports)/pageSize {
		reports = []json.RawMessage{}
	} else {
		start := (page - 1) * pageSize
		if start >= len(reports) {
			reports = []json.RawMessage{}
		} else {
			end := len(reports)
			if pageSize < len(reports)-start {
				end = start + pageSize
			}
			reports = reports[start:end]
		}
	}
	encoded, err := json.Marshal(reports)
	if err != nil {
		return nil, fmt.Errorf("encode report fixture page: %w", err)
	}
	if err := rejectUnexpectedQuery(query, "period_id", "status", "page", "page_size"); err != nil {
		return nil, err
	}
	return encoded, nil
}

func reportVersionData(data json.RawMessage, key string) (json.RawMessage, error) {
	var fixture reportMaterialsFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		return nil, fmt.Errorf("decode report material fixture: %w", err)
	}
	version, ok := fixture.Versions[key]
	if !ok {
		return nil, newFixtureRequestError(http.StatusNotFound, "report version is not present in this fixture")
	}
	return append(json.RawMessage(nil), version...), nil
}

func positiveQueryInt(query url.Values, name string, fallback int) (int, error) {
	value := query.Get(name)
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, newFixtureRequestError(http.StatusBadRequest, name+" must be a positive integer")
	}
	return parsed, nil
}

func requiredPositiveQueryInt64(query url.Values, name string) (int64, error) {
	values, ok := query[name]
	if !ok || len(values) != 1 || values[0] == "" {
		return 0, newFixtureRequestError(http.StatusBadRequest, name+" is required")
	}
	parsed, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || parsed <= 0 {
		return 0, newFixtureRequestError(http.StatusBadRequest, name+" must be a positive integer")
	}
	return parsed, nil
}

func matchOptionalInt64Query(query url.Values, name string, expected *int64) error {
	values, provided := query[name]
	if expected == nil {
		if provided {
			return newFixtureRequestError(http.StatusBadRequest, name+" must be omitted for the selected fixture")
		}
		return nil
	}
	if !provided || len(values) != 1 {
		return newFixtureRequestError(http.StatusBadRequest, name+" does not match the selected fixture")
	}
	parsed, err := strconv.ParseInt(values[0], 10, 64)
	if err != nil || parsed != *expected {
		return newFixtureRequestError(http.StatusBadRequest, name+" does not match the selected fixture")
	}
	return nil
}

func matchOptionalFloatQuery(query url.Values, name string, expected *float64) error {
	values, provided := query[name]
	if expected == nil {
		if provided {
			return newFixtureRequestError(http.StatusBadRequest, name+" must be omitted for the selected fixture")
		}
		return nil
	}
	if !provided || len(values) != 1 {
		return newFixtureRequestError(http.StatusBadRequest, name+" does not match the selected fixture")
	}
	parsed, err := strconv.ParseFloat(values[0], 64)
	if err != nil || parsed != *expected {
		return newFixtureRequestError(http.StatusBadRequest, name+" does not match the selected fixture")
	}
	return nil
}

func rejectUnexpectedQuery(query url.Values, allowed ...string) error {
	allowedSet := stringSet(allowed)
	for key, values := range query {
		if _, ok := allowedSet[key]; !ok {
			return newFixtureRequestError(http.StatusBadRequest, "unexpected query parameter "+key)
		}
		if len(values) != 1 {
			return newFixtureRequestError(http.StatusBadRequest, "query parameter "+key+" must appear once")
		}
	}
	return nil
}

func decodeStrictJSON(data json.RawMessage, target any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
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

type fixtureRequestError struct {
	status  int
	message string
}

func newFixtureRequestError(status int, message string) *fixtureRequestError {
	return &fixtureRequestError{status: status, message: message}
}

func (e *fixtureRequestError) Error() string { return e.message }

func readFixtureBody(body io.Reader) (json.RawMessage, error) {
	if body == nil {
		return nil, nil
	}
	data, err := io.ReadAll(io.LimitReader(body, maxFixtureRequestBody+1))
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(data) > maxFixtureRequestBody {
		return nil, errors.New("request body exceeds fixture limit")
	}
	if len(data) == 0 {
		return nil, nil
	}
	return append(json.RawMessage(nil), data...), nil
}

func writeFixtureSuccess(w http.ResponseWriter, data json.RawMessage) {
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"code":0,"message":"success","data":`))
	_, _ = w.Write(data)
	_, _ = w.Write([]byte("}\n"))
}

func writeFixtureError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    any    `json:"data"`
	}{Code: status, Message: message, Data: nil})
}

func cloneCaseLedger(source CaseLedger) CaseLedger {
	result := CaseLedger{
		CaseID:          source.CaseID,
		Requests:        make([]LedgerRequest, len(source.Requests)),
		ToolCalls:       make(map[string]int, len(source.ToolCalls)),
		WriteAttempts:   source.WriteAttempts,
		WriteSucceeded:  source.WriteSucceeded,
		IdempotencyKeys: append([]string(nil), source.IdempotencyKeys...),
	}
	for index, request := range source.Requests {
		request.Query = cloneValues(request.Query)
		request.Body = append(json.RawMessage(nil), request.Body...)
		result.Requests[index] = request
	}
	for toolName, count := range source.ToolCalls {
		result.ToolCalls[toolName] = count
	}
	return result
}

func cloneValues(source url.Values) url.Values {
	if source == nil {
		return nil
	}
	result := make(url.Values, len(source))
	for key, values := range source {
		result[key] = append([]string(nil), values...)
	}
	return result
}
