package config

import (
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
		MaxExtractedRunes: 500000,
		WorkerInterval:    5 * time.Second,
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
}
