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
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
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
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Description  string `json:"description,omitempty"`
	URL          string `json:"url,omitempty"`
	CategoryName string `json:"category_name"`
	CreatedAt    string `json:"created_at"`
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
	params.Set("search", query)
	var result []KnowledgeItem
	err := c.do(ctx, http.MethodGet, "/api/knowledge/items", params, nil, &result)
	return result, err
}

func (c *Client) CheckinStats(ctx context.Context, period string) ([]CheckinStat, error) {
	params := url.Values{}
	params.Set("period", period)
	var result []CheckinStat
	err := c.do(ctx, http.MethodGet, "/api/checkin/stats", params, nil, &result)
	return result, err
}

type apiResponse struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (c *Client) do(ctx context.Context, method, path string, query url.Values, extraHeaders map[string]string, output any) error {
	requestURL := c.baseURL + path
	if len(query) > 0 {
		requestURL += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, requestURL, bytes.NewReader(nil))
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return fmt.Errorf("read business API response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("business API returned HTTP %d", resp.StatusCode)
	}
	var envelope apiResponse
	if err := json.Unmarshal(body, &envelope); err != nil {
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
