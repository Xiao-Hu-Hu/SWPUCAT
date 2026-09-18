package config

import (
	"math"
	"strings"
	"testing"
	"time"
)

func validRetrievalConfig() RetrievalConfig {
	return RetrievalConfig{
		Mode:              "hybrid",
		CandidateLimit:    20,
		FinalLimit:        8,
		MinVectorScore:    0.3,
		ChunkRunes:        600,
		ChunkOverlapRunes: 80,
		MaxChunks:         10000,
		MaxExtractedRunes: 500000,
		WorkerInterval:    5 * time.Second,
	}
}

func TestAgentConfigRejectsInvalidCostPrices(t *testing.T) {
	base := AgentConfig{Port: 8090, MaxIterations: 4, MaxToolCalls: 8, WorkerConcurrency: 1}
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), 1_000_001} {
		cfg := base
		cfg.InputUSDPerMillion = value
		if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "input_usd_per_million") {
			t.Fatalf("price %v should fail, got %v", value, err)
		}
	}
}

func TestEmbeddingConfigValidate(t *testing.T) {
	cfg := EmbeddingConfig{
		Enabled: true, Provider: "openai_compatible", Model: "embedding-model",
		APIKey: "secret", BaseURL: "https://example.test/v1", Dimensions: 1024,
		BatchSize: 16, Timeout: 30 * time.Second,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid embedding config rejected: %v", err)
	}
	cfg.Model = ""
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "model") {
		t.Fatalf("missing model should fail, got %v", err)
	}
}

func TestAgentConfigRequiresValidLLMBaseURL(t *testing.T) {
	base := AgentConfig{
		Enabled:           true,
		Port:              8090,
		LLMProvider:       "openai_compatible",
		LLMModel:          "chat-model",
		LLMAPIKey:         "secret",
		LLMTimeout:        time.Minute,
		MaxIterations:     4,
		MaxToolCalls:      8,
		WorkerConcurrency: 1,
		InternalSecret:    strings.Repeat("s", 32),
	}

	for _, tc := range []struct {
		name    string
		baseURL string
	}{
		{name: "missing", baseURL: ""},
		{name: "wrong scheme", baseURL: "ftp://example.test/v1"},
		{name: "missing host", baseURL: "https:///v1"},
		{name: "malformed", baseURL: "://example.test/v1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			cfg.LLMBaseURL = tc.baseURL
			if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "llm_base_url") {
				t.Fatalf("invalid base URL should fail validation, got %v", err)
			}
		})
	}

	for _, provider := range []string{"openai", "openai_compatible"} {
		t.Run(provider, func(t *testing.T) {
			cfg := base
			cfg.LLMProvider = provider
			cfg.LLMBaseURL = "https://example.test/v1"
			if err := cfg.Validate(); err != nil {
				t.Fatalf("valid %s config rejected: %v", provider, err)
			}
		})
	}
}

func TestRetrievalConfigRequiresEmbeddingForVectorModes(t *testing.T) {
	cfg := validRetrievalConfig()
	if err := cfg.Validate(true); err != nil {
		t.Fatalf("valid hybrid config rejected: %v", err)
	}
	if err := cfg.Validate(false); err == nil || !strings.Contains(err.Error(), "requires embedding") {
		t.Fatalf("hybrid without embedding should fail, got %v", err)
	}
	cfg.Mode = "keyword"
	if err := cfg.Validate(false); err != nil {
		t.Fatalf("keyword fallback should work without embedding: %v", err)
	}
}

func TestRetrievalConfigRejectsUnsafeChunkLimits(t *testing.T) {
	cfg := validRetrievalConfig()
	cfg.ChunkOverlapRunes = cfg.ChunkRunes
	if err := cfg.Validate(true); err == nil {
		t.Fatal("overlap equal to chunk size should fail")
	}
	cfg = validRetrievalConfig()
	cfg.ChunkOverlapRunes = 301
	if err := cfg.Validate(true); err == nil {
		t.Fatal("overlap above half the chunk size should fail")
	}
}
