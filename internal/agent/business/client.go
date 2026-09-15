package business

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type requestContextKey struct{}

type RequestContext struct {
	Authorization string
	UserID        int64
	Role          string
	RequestID     string
}

func WithRequestContext(ctx context.Context, value RequestContext) context.Context {
	return context.WithValue(ctx, requestContextKey{}, value)
}

func RequestContextFrom(ctx context.Context) RequestContext {
	value, _ := ctx.Value(requestContextKey{}).(RequestContext)
	return value
}

type Client struct {
	baseURL        string
	internalSecret string
	httpClient     *http.Client
}

func NewClient(baseURL, internalSecret string, timeout time.Duration) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("business server URL is required")
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return nil, fmt.Errorf("business server URL is invalid")
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return &Client{baseURL: baseURL, internalSecret: internalSecret, httpClient: &http.Client{Timeout: timeout}}, nil
}

type CheckinStatus struct {
	Status   string `json:"status"`
	ClockIn  string `json:"clock_in,omitempty"`
	ClockOut string `json:"clock_out,omitempty"`
}

type KnowledgeItem struct {
	ID           int64   `json:"id"`
	Type         string  `json:"type"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	URL          string  `json:"url,omitempty"`
	DownloadURL  string  `json:"download_url,omitempty"`
	CategoryName string  `json:"category_name"`
	SourceKind   string  `json:"source_kind,omitempty"`
	Excerpt      string  `json:"excerpt,omitempty"`
	Score        float64 `json:"score,omitempty"`
	MatchMode    string  `json:"match_mode,omitempty"`
	Status       string  `json:"status,omitempty"`
	CreatedAt    string  `json:"created_at,omitempty"`
}

type KnowledgeRecommendation struct {
	ItemID          int64    `json:"item_id"`
	Name            string   `json:"name"`
	Description     string   `json:"description,omitempty"`
	URL             string   `json:"url,omitempty"`
	CategoryName    string   `json:"category_name"`
	SummaryMarkdown string   `json:"summary_markdown,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Reason          string   `json:"reason"`
}

// KnowledgeSource is the safe, user-facing projection of a knowledge item.
// It deliberately excludes FileKey and other storage details.
type KnowledgeSource struct {
	ID           int64   `json:"id"`
	Type         string  `json:"type"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	URL          string  `json:"url,omitempty"`
	DownloadURL  string  `json:"download_url,omitempty"`
	CategoryName string  `json:"category_name"`
	SourceKind   string  `json:"source_kind,omitempty"`
	Excerpt      string  `json:"excerpt,omitempty"`
	Score        float64 `json:"score,omitempty"`
	MatchMode    string  `json:"match_mode,omitempty"`
	CreatedAt    string  `json:"created_at,omitempty"`
}

const (
	maxKnowledgeSources          = 8
	maxKnowledgeDescriptionRunes = 4000
	maxKnowledgeExcerptRunes     = 600
)

func BuildKnowledgeSources(items []KnowledgeItem) []KnowledgeSource {
	result := make([]KnowledgeSource, 0, minInt(len(items), maxKnowledgeSources))
	seen := make(map[int64]struct{}, maxKnowledgeSources)
	for _, item := range items {
		if len(result) >= maxKnowledgeSources {
			break
		}
		if item.ID != 0 {
			if _, ok := seen[item.ID]; ok {
				continue
			}
			seen[item.ID] = struct{}{}
		}
		item.Description = truncateRunes(item.Description, maxKnowledgeDescriptionRunes)
		result = append(result, KnowledgeSource{
			ID:           item.ID,
			Type:         item.Type,
			Name:         item.Name,
			Description:  item.Description,
			URL:          item.URL,
			DownloadURL:  item.DownloadURL,
			CategoryName: item.CategoryName,
			SourceKind:   item.SourceKind,
			Excerpt:      truncateRunes(item.Excerpt, maxKnowledgeExcerptRunes),
			Score:        item.Score,
			MatchMode:    item.MatchMode,
			CreatedAt:    item.CreatedAt,
		})
	}
	return result
}

func KnowledgeSourceExcerpt(source KnowledgeSource) KnowledgeSource {
	source.Description = truncateRunes(source.Description, maxKnowledgeExcerptRunes)
	source.Excerpt = truncateRunes(source.Excerpt, maxKnowledgeExcerptRunes)
	return source
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
}

type sourceCollectorContextKey struct{}

// KnowledgeSourceCollector deduplicates sources observed during one chat.
type KnowledgeSourceCollector struct {
	mu      sync.Mutex
	sources []KnowledgeSource
	seen    map[int64]struct{}
}

func NewKnowledgeSourceCollector() *KnowledgeSourceCollector {
	return &KnowledgeSourceCollector{seen: make(map[int64]struct{}, maxKnowledgeSources)}
}

func WithKnowledgeSourceCollector(ctx context.Context, collector *KnowledgeSourceCollector) context.Context {
	return context.WithValue(ctx, sourceCollectorContextKey{}, collector)
}

func KnowledgeSourceCollectorFrom(ctx context.Context) *KnowledgeSourceCollector {
	collector, _ := ctx.Value(sourceCollectorContextKey{}).(*KnowledgeSourceCollector)
	return collector
}

func (c *KnowledgeSourceCollector) Add(items []KnowledgeItem) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, source := range BuildKnowledgeSources(items) {
		if len(c.sources) >= maxKnowledgeSources {
			return
		}
		if source.ID != 0 {
			if _, ok := c.seen[source.ID]; ok {
				continue
			}
			c.seen[source.ID] = struct{}{}
		}
		c.sources = append(c.sources, KnowledgeSourceExcerpt(source))
	}
}

func (c *KnowledgeSourceCollector) AddSources(items []KnowledgeSource) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, source := range items {
		if len(c.sources) >= maxKnowledgeSources {
			return
		}
		if source.ID != 0 {
			if _, ok := c.seen[source.ID]; ok {
				continue
			}
			c.seen[source.ID] = struct{}{}
		}
		c.sources = append(c.sources, KnowledgeSourceExcerpt(source))
	}
}

func (c *KnowledgeSourceCollector) List() []KnowledgeSource {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]KnowledgeSource(nil), c.sources...)
}

type CheckinStat struct {
	UserID       int64    `json:"user_id"`
	Nickname     string   `json:"nickname"`
	TotalMinutes float64  `json:"total_minutes"`
	TotalHours   float64  `json:"total_hours"`
	Days         int      `json:"days"`
	Status       string   `json:"status"`
	Anomalies    []string `json:"anomalies,omitempty"`
}

type TeamQueryMember struct {
	UserID          int64    `json:"user_id"`
	Nickname        string   `json:"nickname"`
	TotalHours      float64  `json:"total_hours"`
	Days            int      `json:"days"`
	AttendanceState string   `json:"attendance_state"`
	Anomalies       []string `json:"anomalies,omitempty"`
	ReportState     string   `json:"report_state,omitempty"`
	Reasons         []string `json:"reasons"`
}

type TeamQueryResult struct {
	Period       string            `json:"period"`
	PeriodID     int64             `json:"period_id,omitempty"`
	MinHours     *float64          `json:"min_hours,omitempty"`
	ReportStatus string            `json:"report_status,omitempty"`
	Limit        int               `json:"limit"`
	Members      []TeamQueryMember `json:"members"`
}

type Report struct {
	ID             int64  `json:"id"`
	PeriodID       int64  `json:"period_id"`
	ReportType     string `json:"report_type"`
	AuthorID       int64  `json:"author_id"`
	AuthorName     string `json:"author_name"`
	Status         string `json:"status"`
	Visibility     string `json:"visibility"`
	CurrentVersion int    `json:"current_version"`
}

type ReportVersion struct {
	ID           int64          `json:"id"`
	ReportID     int64          `json:"report_id"`
	Version      int            `json:"version"`
	Status       string         `json:"status"`
	Title        string         `json:"title"`
	BodyMarkdown string         `json:"body_markdown"`
	ParseStatus  string         `json:"parse_status"`
	ParseError   string         `json:"parse_error,omitempty"`
	ParsedJSON   map[string]any `json:"parsed_json,omitempty"`
	Assets       []ReportAsset  `json:"assets,omitempty"`
}

type ReportAsset struct {
	ReportVersionID int64  `json:"report_version_id"`
	KnowledgeItemID int64  `json:"knowledge_item_id"`
	AssetRole       string `json:"asset_role"`
	Locator         string `json:"locator,omitempty"`
	Note            string `json:"note,omitempty"`
}

type AnnouncementDraftRequest struct {
	Title          string         `json:"title"`
	Content        string         `json:"content"`
	Pinned         bool           `json:"pinned"`
	Source         map[string]any `json:"source,omitempty"`
	RecipientIDs   []int64        `json:"recipient_ids,omitempty"`
	EmailRequested bool           `json:"email_requested"`
}

type AnnouncementDraft struct {
	ID              int64   `json:"id"`
	Status          string  `json:"status"`
	Title           string  `json:"title"`
	Content         string  `json:"content"`
	Pinned          bool    `json:"pinned"`
	SourceJSON      string  `json:"source_json,omitempty"`
	RecipientIDs    []int64 `json:"recipient_ids"`
	EmailRequested  bool    `json:"email_requested"`
	EmailResultJSON string  `json:"email_result_json,omitempty"`
	AnnouncementID  int64   `json:"announcement_id,omitempty"`
	CreatedBy       int64   `json:"created_by"`
	CreatedAt       string  `json:"created_at,omitempty"`
	UpdatedAt       string  `json:"updated_at,omitempty"`
	ConfirmedAt     string  `json:"confirmed_at,omitempty"`
	PublishedAt     string  `json:"published_at,omitempty"`
}

func (c *Client) CheckinStatus(ctx context.Context) (CheckinStatus, error) {
	var result CheckinStatus
	err := c.do(ctx, http.MethodGet, "/api/checkin/status", nil, nil, &result)
	return result, err
}

func (c *Client) ClockIn(ctx context.Context, idempotencyKey string) (map[string]any, error) {
	var result map[string]any
	headers := map[string]string{}
	if idempotencyKey != "" {
		headers["Idempotency-Key"] = idempotencyKey
	}
	err := c.do(ctx, http.MethodPost, "/api/checkin/clock-in", nil, headers, &result)
	return result, err
}

func (c *Client) ClockOut(ctx context.Context, idempotencyKey string) (map[string]any, error) {
	var result map[string]any
	headers := map[string]string{}
	if idempotencyKey != "" {
		headers["Idempotency-Key"] = idempotencyKey
	}
	err := c.do(ctx, http.MethodPost, "/api/checkin/clock-out", nil, headers, &result)
	return result, err
}

func (c *Client) SearchKnowledge(ctx context.Context, query string) ([]KnowledgeItem, error) {
	params := url.Values{}
	params.Set("q", query)
	var result []KnowledgeItem
	err := c.do(ctx, http.MethodGet, "/api/knowledge/search", params, nil, &result)
	return result, err
}

func (c *Client) RecommendKnowledge(ctx context.Context, query string) ([]KnowledgeRecommendation, error) {
	params := url.Values{}
	params.Set("q", query)
	var result []KnowledgeRecommendation
	err := c.do(ctx, http.MethodGet, "/api/knowledge/recommendations", params, nil, &result)
	return result, err
}

func (c *Client) CheckinStats(ctx context.Context, period string) ([]CheckinStat, error) {
	params := url.Values{}
	params.Set("period", period)
	var result []CheckinStat
	err := c.do(ctx, http.MethodGet, "/api/checkin/stats", params, nil, &result)
	return result, err
}

func (c *Client) QueryTeam(ctx context.Context, period string, periodID int64, minHours *float64, reportStatus string, limit int) (TeamQueryResult, error) {
	params := url.Values{}
	params.Set("period", period)
	if periodID > 0 {
		params.Set("period_id", fmt.Sprintf("%d", periodID))
	}
	if minHours != nil {
		params.Set("min_hours", fmt.Sprintf("%g", *minHours))
	}
	if reportStatus != "" {
		params.Set("report_status", reportStatus)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	var result TeamQueryResult
	err := c.do(ctx, http.MethodGet, "/api/analysis/team", params, nil, &result)
	return result, err
}

func (c *Client) ListReports(ctx context.Context, periodID int64, status string) ([]Report, error) {
	const pageSize = 100
	const maxPages = 100
	result := make([]Report, 0)
	for page := 1; page <= maxPages; page++ {
		params := url.Values{}
		if periodID > 0 {
			params.Set("period_id", fmt.Sprintf("%d", periodID))
		}
		if status != "" {
			params.Set("status", status)
		}
		params.Set("page", fmt.Sprintf("%d", page))
		params.Set("page_size", fmt.Sprintf("%d", pageSize))
		var batch []Report
		if err := c.do(ctx, http.MethodGet, "/api/reports", params, nil, &batch); err != nil {
			return nil, err
		}
		result = append(result, batch...)
		if len(batch) < pageSize {
			break
		}
	}
	return result, nil
}

func (c *Client) GetReportVersion(ctx context.Context, reportID int64, version int) (ReportVersion, error) {
	var result ReportVersion
	err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/reports/%d/versions/%d", reportID, version), nil, nil, &result)
	return result, err
}

func (c *Client) CreateAnnouncementDraft(ctx context.Context, req AnnouncementDraftRequest) (AnnouncementDraft, error) {
	var result AnnouncementDraft
	err := c.doJSON(ctx, http.MethodPost, "/api/announcement-drafts", nil, nil, req, &result)
	return result, err
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, extraHeaders map[string]string, output any) error {
	return c.doWithBody(ctx, method, path, query, extraHeaders, nil, output)
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, extraHeaders map[string]string, payload any, output any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("encode business API request: %w", err)
	}
	return c.doWithBody(ctx, method, path, query, extraHeaders, body, output)
}

func (c *Client) doWithBody(ctx context.Context, method, path string, query url.Values, extraHeaders map[string]string, body []byte, output any) error {
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if len(body) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	if identity := RequestContextFrom(ctx); identity.Authorization != "" {
		req.Header.Set("Authorization", identity.Authorization)
	}
	if c.internalSecret != "" {
		req.Header.Set("X-Agent-Internal-Secret", c.internalSecret)
	}
	for key, value := range extraHeaders {
		req.Header.Set(key, value)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("business API request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read business API response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("business API returned HTTP %d", resp.StatusCode)
	}
	var envelope apiResponse
	if err := json.Unmarshal(responseBody, &envelope); err != nil {
		return fmt.Errorf("decode business API response: %w", err)
	}
	if envelope.Code != 0 {
		return fmt.Errorf("business API error %d: %s", envelope.Code, envelope.Message)
	}
	if output == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return fmt.Errorf("decode business API data: %w", err)
	}
	return nil
}
