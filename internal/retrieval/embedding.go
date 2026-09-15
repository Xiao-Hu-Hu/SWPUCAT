// Package retrieval contains reusable primitives for knowledge retrieval.
package retrieval

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrEmbeddingBaseURL    = errors.New("embedding base URL is invalid")
	ErrEmbeddingModel      = errors.New("embedding model is required")
	ErrEmbeddingDimensions = errors.New("embedding dimensions must be positive")
	ErrEmbeddingInput      = errors.New("embedding input is required")
)

const (
	defaultEmbeddingTimeout   = 20 * time.Second
	defaultEmbeddingBatchSize = 32
	maxEmbeddingResponseBytes = 8 << 20
)

// EmbeddingClientConfig describes an OpenAI-compatible /embeddings endpoint.
// BaseURL can be either a version root (for example https://host/v1) or the
// full https://host/v1/embeddings endpoint.
type EmbeddingClientConfig struct {
	BaseURL      string
	APIKey       string
	Model        string
	Dimensions   int
	Timeout      time.Duration
	MaxBatchSize int
	HTTPClient   *http.Client
}

// EmbeddingClient invokes an OpenAI-compatible embedding API. It is safe for
// concurrent use as http.Client is safe for concurrent use.
type EmbeddingClient struct {
	endpoint     string
	apiKey       string
	model        string
	dimensions   int
	maxBatchSize int
	httpClient   *http.Client
}

func NewEmbeddingClient(cfg EmbeddingClientConfig) (*EmbeddingClient, error) {
	endpoint, err := embeddingEndpoint(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, ErrEmbeddingModel
	}
	if cfg.Dimensions <= 0 {
		return nil, ErrEmbeddingDimensions
	}
	batchSize := cfg.MaxBatchSize
	if batchSize <= 0 {
		batchSize = defaultEmbeddingBatchSize
	}
	client := cfg.HTTPClient
	if client == nil {
		timeout := cfg.Timeout
		if timeout <= 0 {
			timeout = defaultEmbeddingTimeout
		}
		client = &http.Client{Timeout: timeout}
	}
	return &EmbeddingClient{
		endpoint: endpoint, apiKey: strings.TrimSpace(cfg.APIKey),
		model: strings.TrimSpace(cfg.Model), dimensions: cfg.Dimensions,
		maxBatchSize: batchSize, httpClient: client,
	}, nil
}

func embeddingEndpoint(baseURL string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", ErrEmbeddingBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", ErrEmbeddingBaseURL
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if !strings.HasSuffix(parsed.Path, "/embeddings") {
		parsed.Path += "/embeddings"
	}
	return parsed.String(), nil
}

// Embed creates one embedding. It is equivalent to a single-item batch.
func (c *EmbeddingClient) Embed(ctx context.Context, input string) ([]float32, error) {
	vectors, err := c.EmbedBatch(ctx, []string{input})
	if err != nil {
		return nil, err
	}
	return vectors[0], nil
}

// EmbedBatch embeds every input in order. Inputs are split into bounded API
// requests, and API response indexes are checked before they are returned.
func (c *EmbeddingClient) EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return [][]float32{}, nil
	}
	for _, input := range inputs {
		if strings.TrimSpace(input) == "" {
			return nil, ErrEmbeddingInput
		}
	}
	result := make([][]float32, 0, len(inputs))
	for start := 0; start < len(inputs); start += c.maxBatchSize {
		end := start + c.maxBatchSize
		if end > len(inputs) {
			end = len(inputs)
		}
		batch, err := c.embedBatch(ctx, inputs[start:end])
		if err != nil {
			return nil, fmt.Errorf("embed batch %d-%d: %w", start, end, err)
		}
		result = append(result, batch...)
	}
	return result, nil
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float64 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
}

func (c *EmbeddingClient) embedBatch(ctx context.Context, inputs []string) ([][]float32, error) {
	payload, err := json.Marshal(embeddingRequest{Model: c.model, Input: inputs})
	if err != nil {
		return nil, fmt.Errorf("encode embedding request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("create embedding request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding API request: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxEmbeddingResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read embedding response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("embedding API returned HTTP %d: %s", resp.StatusCode, responseErrorMessage(body))
	}
	var decoded embeddingResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return nil, fmt.Errorf("decode embedding response: %w", err)
	}
	if len(decoded.Data) != len(inputs) {
		return nil, fmt.Errorf("embedding API returned %d vectors for %d inputs", len(decoded.Data), len(inputs))
	}
	result := make([][]float32, len(inputs))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(inputs) || result[item.Index] != nil {
			return nil, fmt.Errorf("embedding API returned an invalid or duplicate index %d", item.Index)
		}
		vector := make([]float32, len(item.Embedding))
		for i, value := range item.Embedding {
			if math.IsNaN(value) || math.IsInf(value, 0) || value > math.MaxFloat32 || value < -math.MaxFloat32 {
				return nil, fmt.Errorf("embedding API returned a non-finite value at index %d", item.Index)
			}
			vector[i] = float32(value)
		}
		if err := ValidateVector(vector, c.dimensions); err != nil {
			return nil, fmt.Errorf("embedding API returned invalid vector at index %d: %w", item.Index, err)
		}
		result[item.Index] = vector
	}
	for index, vector := range result {
		if vector == nil {
			return nil, fmt.Errorf("embedding API did not return vector at index %d", index)
		}
	}
	return result, nil
}

func responseErrorMessage(body []byte) string {
	var decoded struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &decoded) == nil {
		if message := strings.TrimSpace(decoded.Error.Message); message != "" {
			return message
		}
		if message := strings.TrimSpace(decoded.Message); message != "" {
			return message
		}
	}
	message := strings.TrimSpace(string(body))
	if len(message) > 512 {
		message = message[:512] + "…"
	}
	return message
}
