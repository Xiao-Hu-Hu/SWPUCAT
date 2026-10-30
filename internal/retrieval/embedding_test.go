package retrieval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmbeddingClientEmbedBatchPreservesResponseIndexesAndBatches(t *testing.T) {
	var requestCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.URL.Path != "/v1/embeddings" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("Authorization = %q", got)
		}
		var request embeddingRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Model != "embed-test" {
			t.Fatalf("model = %q", request.Model)
		}
		data := make([]map[string]any, 0, len(request.Input))
		for index := len(request.Input) - 1; index >= 0; index-- {
			data = append(data, map[string]any{"index": index, "embedding": []float64{float64(index + 1), 2}})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()

	client, err := NewEmbeddingClient(EmbeddingClientConfig{
		BaseURL: server.URL + "/v1", APIKey: "test-key", Model: "embed-test", Dimensions: 2, MaxBatchSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	vectors, err := client.EmbedBatch(context.Background(), []string{"one", "two", "three"})
	if err != nil {
		t.Fatal(err)
	}
	if requestCount != 2 {
		t.Fatalf("requests = %d, want 2", requestCount)
	}
	if len(vectors) != 3 || vectors[0][0] != 1 || vectors[1][0] != 2 || vectors[2][0] != 1 {
		t.Fatalf("unexpected vectors: %#v", vectors)
	}
}

func TestEmbeddingClientRejectsMalformedResponses(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{name: "missing vector", body: `{"data":[]}`},
		{name: "duplicate indexes", body: `{"data":[{"index":0,"embedding":[1,2]},{"index":0,"embedding":[1,2]}]}`},
		{name: "wrong dimensions", body: `{"data":[{"index":0,"embedding":[1]}]}`},
		{name: "non finite", body: `{"data":[{"index":0,"embedding":[1e999,2]}]}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(test.body))
			}))
			defer server.Close()
			client, err := NewEmbeddingClient(EmbeddingClientConfig{BaseURL: server.URL, Model: "m", Dimensions: 2})
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.EmbedBatch(context.Background(), []string{"one", "two"})
			if err == nil {
				t.Fatal("expected malformed response error")
			}
		})
	}
}

func TestEmbeddingClientReturnsAPIErrorsAndHonorsTimeout(t *testing.T) {
	t.Run("API error message", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"capacity exhausted"}}`))
		}))
		defer server.Close()
		client, err := NewEmbeddingClient(EmbeddingClientConfig{BaseURL: server.URL, Model: "m", Dimensions: 2})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Embed(context.Background(), "hello")
		if err == nil || !strings.Contains(err.Error(), "HTTP 429: capacity exhausted") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
		}))
		defer server.Close()
		client, err := NewEmbeddingClient(EmbeddingClientConfig{BaseURL: server.URL, Model: "m", Dimensions: 2, Timeout: 10 * time.Millisecond})
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Embed(context.Background(), "hello")
		if err == nil {
			t.Fatal("expected timeout")
		}
	})
}

func TestEmbeddingClientValidatesConfigurationAndInput(t *testing.T) {
	_, err := NewEmbeddingClient(EmbeddingClientConfig{BaseURL: "ftp://example.test", Model: "m", Dimensions: 2})
	if !errors.Is(err, ErrEmbeddingBaseURL) {
		t.Fatalf("base URL error = %v", err)
	}
	_, err = NewEmbeddingClient(EmbeddingClientConfig{BaseURL: "https://example.test", Dimensions: 2})
	if !errors.Is(err, ErrEmbeddingModel) {
		t.Fatalf("model error = %v", err)
	}
	client, err := NewEmbeddingClient(EmbeddingClientConfig{BaseURL: "https://example.test/v1/embeddings", Model: "m", Dimensions: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.EmbedBatch(context.Background(), []string{"ok", " "})
	if !errors.Is(err, ErrEmbeddingInput) {
		t.Fatalf("input error = %v", err)
	}
}
