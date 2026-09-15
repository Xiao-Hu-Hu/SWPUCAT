package knowledgeindex

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"SWPUCAT/internal/retrieval"
)

const (
	// Bump when extraction or the synthetic description text changes so an
	// upgrade automatically reindexes existing approved sources instead of
	// leaving stale chunks in PostgreSQL.
	indexFormatVersion = "knowledge-index-v2"
	defaultMaxAttempts = 5
	maxQueryRunes      = 512
)

type Config struct {
	Mode                string
	CandidateLimit      int
	FinalLimit          int
	MinVectorScore      float64
	ChunkRunes          int
	ChunkOverlapRunes   int
	MaxChunks           int
	MaxExtractedRunes   int
	WorkerInterval      time.Duration
	EmbeddingModel      string
	EmbeddingDimensions int
	MaxAttempts         int
}

type Service struct {
	repo      Repository
	embedder  Embedder
	extractor TextExtractor
	files     FileStore
	config    Config
}

func NewService(repo Repository, embedder Embedder, extractor TextExtractor, files FileStore, cfg Config) (*Service, error) {
	if repo == nil {
		return nil, errors.New("knowledge index repository is required")
	}
	if cfg.Mode != "keyword" && cfg.Mode != "vector" && cfg.Mode != "hybrid" {
		return nil, fmt.Errorf("invalid retrieval mode %q", cfg.Mode)
	}
	if (cfg.Mode == "vector" || cfg.Mode == "hybrid") && embedder == nil {
		return nil, errors.New("vector retrieval requires an embedder")
	}
	if cfg.CandidateLimit < 1 || cfg.FinalLimit < 1 || cfg.FinalLimit > cfg.CandidateLimit {
		return nil, errors.New("invalid retrieval result limits")
	}
	if cfg.ChunkRunes < 1 || cfg.ChunkOverlapRunes < 0 || cfg.ChunkOverlapRunes >= cfg.ChunkRunes {
		return nil, errors.New("invalid knowledge chunk limits")
	}
	if cfg.MaxChunks < 1 || cfg.MaxChunks > 50000 {
		return nil, errors.New("invalid knowledge max chunks")
	}
	if cfg.MaxExtractedRunes < cfg.ChunkRunes {
		return nil, errors.New("max extracted runes must be at least one chunk")
	}
	if embedder != nil && (strings.TrimSpace(cfg.EmbeddingModel) == "" || cfg.EmbeddingDimensions <= 0) {
		return nil, errors.New("embedding model and dimensions are required")
	}
	if cfg.WorkerInterval <= 0 {
		cfg.WorkerInterval = 5 * time.Second
	}
	if cfg.MaxAttempts <= 0 {
		cfg.MaxAttempts = defaultMaxAttempts
	}
	return &Service{repo: repo, embedder: embedder, extractor: extractor, files: files, config: cfg}, nil
}

// Reconcile schedules every approved source whose metadata, chunk settings or
// embedding model differs from its completed index fingerprint.
func (s *Service) Reconcile(ctx context.Context) (int, error) {
	sources, err := s.repo.ListApprovedSources(ctx)
	if err != nil {
		return 0, err
	}
	for _, source := range sources {
		if err := s.repo.EnsureJob(ctx, source.ID, s.fingerprint(source)); err != nil {
			return 0, err
		}
	}
	return len(sources), nil
}

// RunOnce claims and handles at most one durable job. The bool reports whether
// a job was claimed, allowing callers to drain a small backlog without polling.
func (s *Service) RunOnce(ctx context.Context) (bool, error) {
	now := time.Now()
	job, err := s.repo.ClaimJob(ctx, now, now.Add(-5*time.Minute), s.config.MaxAttempts)
	if errors.Is(err, ErrNoPendingJob) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	finalFingerprint := s.fingerprint(job.Source)
	chunks, processErr := s.buildChunks(ctx, job.Source)
	if processErr != nil {
		// Keep the description searchable even when an attachment cannot be
		// opened, parsed, or embedded. The partial job retains its expected
		// fingerprint, records a retry/backoff state, and leaves keyword
		// retrieval available in the meantime.
		if len(chunks) > 0 {
			retryAt := now.Add(retryDelay(job.Attempts))
			if job.Attempts >= s.config.MaxAttempts {
				retryAt = now.Add(time.Hour)
			}
			if completeErr := s.repo.CompletePartialJob(ctx, job, finalFingerprint, chunks, truncateRunes(processErr.Error(), 900), retryAt, s.config.MaxAttempts); completeErr != nil {
				return true, errors.Join(processErr, completeErr)
			}
			return true, processErr
		}
		retryAt := now.Add(retryDelay(job.Attempts))
		if job.Attempts >= s.config.MaxAttempts {
			retryAt = now.Add(time.Hour)
		}
		message := truncateRunes(processErr.Error(), 900)
		if failErr := s.repo.FailJob(ctx, job, message, retryAt, s.config.MaxAttempts); failErr != nil {
			return true, errors.Join(processErr, failErr)
		}
		return true, processErr
	}
	if err := s.repo.CompleteJob(ctx, job, finalFingerprint, chunks); err != nil {
		return true, err
	}
	return true, nil
}

// Run starts the one-worker loop used by the 2-core/2G deployment. It performs
// startup reconciliation and repeats it periodically so newly approved items
// are eventually indexed even if their request was interrupted after commit.
func (s *Service) Run(ctx context.Context, logf func(string, ...any)) {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	reconcile := func() {
		if _, err := s.Reconcile(ctx); err != nil && !errors.Is(err, context.Canceled) {
			logf("knowledge index reconciliation failed: %v", err)
		}
	}
	drain := func() {
		for processed := 0; processed < 4; processed++ {
			claimed, err := s.RunOnce(ctx)
			if err != nil && !errors.Is(err, context.Canceled) {
				logf("knowledge index job warning: %v", err)
			}
			if !claimed || ctx.Err() != nil {
				return
			}
		}
	}
	reconcile()
	drain()
	workTicker := time.NewTicker(s.config.WorkerInterval)
	reconcileTicker := time.NewTicker(5 * time.Minute)
	defer workTicker.Stop()
	defer reconcileTicker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-workTicker.C:
			drain()
		case <-reconcileTicker.C:
			reconcile()
		}
	}
}

func (s *Service) buildChunks(ctx context.Context, source Source) ([]Chunk, error) {
	type textPart struct {
		kind string
		text string
	}
	parts := []textPart{{kind: "description", text: sourceDescription(source)}}
	buildTextChunks := func(input []textPart) ([]Chunk, error) {
		chunks := make([]Chunk, 0)
		for _, part := range input {
			metadata, _ := json.Marshal(map[string]any{"source_kind": part.kind, "name": source.Name, "category": source.CategoryName})
			remaining := s.config.MaxChunks - len(chunks)
			if remaining <= 0 && strings.TrimSpace(part.text) != "" {
				return nil, errChunkLimit
			}
			partChunks, err := splitTextBounded(part.text, s.config.ChunkRunes, s.config.ChunkOverlapRunes, remaining)
			if err != nil {
				return nil, err
			}
			for index, content := range partChunks {
				digest := sha256.Sum256([]byte(content))
				chunks = append(chunks, Chunk{SourceKind: part.kind, Index: index, Content: content, ContentHash: hex.EncodeToString(digest[:]), MetadataJSON: string(metadata)})
			}
		}
		return chunks, nil
	}
	descriptionChunks := func() []Chunk {
		chunks, _ := buildTextChunks(parts[:1])
		return chunks
	}
	if source.Type == "file" && s.extractor != nil && s.files != nil && supportedFile(source.Name, source.FileKey) {
		file, err := s.files.Open(ctx, source.FileKey)
		if err != nil {
			return descriptionChunks(), fmt.Errorf("open knowledge file: %w", err)
		}
		filename := source.Name
		if filepath.Ext(filename) == "" {
			filename = source.FileKey
		}
		body, extractErr := s.extractor.ExtractText(ctx, filename, file)
		closeErr := file.Close()
		if extractErr != nil {
			return descriptionChunks(), fmt.Errorf("extract knowledge file: %w", extractErr)
		}
		if closeErr != nil {
			return descriptionChunks(), fmt.Errorf("close knowledge file: %w", closeErr)
		}
		body = truncateRunes(body, s.config.MaxExtractedRunes)
		if strings.TrimSpace(body) != "" {
			parts = append(parts, textPart{kind: "content", text: body})
		}
	}

	chunks, chunkErr := buildTextChunks(parts)
	if chunkErr != nil {
		return descriptionChunks(), chunkErr
	}
	if len(chunks) == 0 {
		return nil, errors.New("knowledge source produced no indexable text")
	}
	if s.embedder != nil && s.config.Mode != "keyword" {
		inputs := make([]string, len(chunks))
		for i := range chunks {
			inputs[i] = chunks[i].Content
		}
		vectors, err := s.embedder.EmbedBatch(ctx, inputs)
		if err != nil {
			return chunks, fmt.Errorf("embed knowledge chunks: %w", err)
		}
		if len(vectors) != len(chunks) {
			return chunks, fmt.Errorf("embedding count mismatch: got %d, want %d", len(vectors), len(chunks))
		}
		for i := range chunks {
			if err := retrieval.ValidateVector(vectors[i], s.config.EmbeddingDimensions); err != nil {
				return chunks, fmt.Errorf("validate chunk embedding %d: %w", i, err)
			}
			chunks[i].Embedding = vectors[i]
			chunks[i].EmbeddingModel = s.config.EmbeddingModel
			chunks[i].EmbeddingDimensions = s.config.EmbeddingDimensions
		}
	}
	return chunks, nil
}

func (s *Service) Search(ctx context.Context, query string) ([]SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" || !utf8.ValidString(query) || utf8.RuneCountInString(query) > maxQueryRunes {
		return nil, ErrInvalidQuery
	}
	var keyword, vector []Candidate
	var keywordErr, vectorErr error
	if s.config.Mode == "keyword" || s.config.Mode == "hybrid" {
		keyword, keywordErr = s.repo.KeywordSearch(ctx, query, s.config.CandidateLimit)
	}
	if s.config.Mode == "vector" || s.config.Mode == "hybrid" {
		var embedding []float32
		embedding, vectorErr = s.embedder.Embed(ctx, query)
		if vectorErr == nil {
			vector, vectorErr = s.repo.VectorSearch(ctx, embedding, s.config.EmbeddingModel, s.config.EmbeddingDimensions, s.config.CandidateLimit, s.config.MinVectorScore)
		}
	}
	if s.config.Mode == "keyword" {
		if keywordErr != nil {
			return nil, keywordErr
		}
		return candidatesToResults(keyword, "keyword", s.config.FinalLimit), nil
	}
	if s.config.Mode == "vector" {
		if vectorErr != nil {
			return nil, vectorErr
		}
		return candidatesToResults(vector, "vector", s.config.FinalLimit), nil
	}
	if keywordErr != nil && vectorErr != nil {
		return nil, errors.Join(keywordErr, vectorErr)
	}
	if vectorErr != nil {
		return candidatesToResults(keyword, "keyword", s.config.FinalLimit), nil
	}
	if keywordErr != nil {
		return candidatesToResults(vector, "vector", s.config.FinalLimit), nil
	}
	return fuseCandidates(vector, keyword, s.config.FinalLimit)
}

func (s *Service) fingerprint(source Source) string {
	payload, _ := json.Marshal([]any{
		indexFormatVersion, source.ID, source.Type, source.Name, source.Description,
		source.URL, source.FileKey, source.CategoryName, s.config.ChunkRunes,
		s.config.ChunkOverlapRunes, s.config.MaxChunks, s.config.MaxExtractedRunes,
		s.config.Mode,
		s.config.EmbeddingModel, s.config.EmbeddingDimensions,
	})
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

func sourceDescription(source Source) string {
	var b strings.Builder
	b.WriteString("# ")
	b.WriteString(strings.TrimSpace(source.Name))
	b.WriteString("\n\n")
	if source.CategoryName != "" {
		b.WriteString("分类：")
		b.WriteString(strings.TrimSpace(source.CategoryName))
		b.WriteString("\n\n")
	}
	if source.Type == "link" && source.URL != "" {
		b.WriteString("链接：")
		b.WriteString(strings.TrimSpace(source.URL))
		b.WriteString("\n\n")
	}
	b.WriteString("资料说明：\n")
	description := strings.TrimSpace(source.Description)
	if description == "" {
		description = "未提供说明。"
	}
	b.WriteString(description)
	return b.String()
}

func supportedFile(values ...string) bool {
	for _, value := range values {
		switch strings.ToLower(filepath.Ext(value)) {
		case ".pdf", ".docx", ".xlsx", ".md", ".markdown", ".txt":
			return true
		}
	}
	return false
}

func retryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > 7 {
		attempt = 7
	}
	return time.Duration(1<<(attempt-1)) * 5 * time.Second
}

func fuseCandidates(vector, keyword []Candidate, limit int) ([]SearchResult, error) {
	// Repositories return chunk-level candidates so both descriptions and
	// extracted file content remain available.  Collapse each channel before
	// RRF; otherwise repeated chunks of one item would consume several ranks
	// and the fused payload could still be the description chunk.
	vector = aggregateCandidates(vector)
	keyword = aggregateCandidates(keyword)
	toRanked := func(items []Candidate) []retrieval.RankedItem {
		result := make([]retrieval.RankedItem, 0, len(items))
		for _, item := range items {
			result = append(result, retrieval.RankedItem{ID: strconv.FormatInt(item.ItemID, 10), Payload: item})
		}
		return result
	}
	fused, err := retrieval.FuseRRF([][]retrieval.RankedItem{toRanked(vector), toRanked(keyword)}, retrieval.RRFOptions{Weights: []float64{1, 0.8}, Limit: limit})
	if err != nil {
		return nil, err
	}
	result := make([]SearchResult, 0, len(fused))
	for _, item := range fused {
		candidate, ok := item.Payload.(Candidate)
		if !ok {
			return nil, errors.New("invalid fused knowledge result")
		}
		converted := candidateToResult(candidate, "hybrid")
		converted.Score = item.Score
		result = append(result, converted)
	}
	return result, nil
}

func candidatesToResults(items []Candidate, mode string, limit int) []SearchResult {
	// Collapse only at the API boundary.  Content is preferred as the excerpt
	// while the original item description remains in Candidate.Description.
	items = aggregateCandidates(items)
	if len(items) > limit {
		items = items[:limit]
	}
	result := make([]SearchResult, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for _, item := range items {
		if _, exists := seen[item.ItemID]; exists {
			continue
		}
		seen[item.ItemID] = struct{}{}
		result = append(result, candidateToResult(item, mode))
	}
	return result
}

func candidateToResult(item Candidate, mode string) SearchResult {
	result := SearchResult{
		ItemID: item.ItemID, Type: item.Type, Name: item.Name,
		Description: truncateRunes(item.Description, 2000), URL: item.URL,
		CategoryName: item.CategoryName, SourceKind: item.SourceKind,
		Excerpt: truncateRunes(item.Excerpt, 800), Score: item.Score, MatchMode: mode,
	}
	if item.Type == "file" {
		result.DownloadURL = "/api/knowledge/download/" + strconv.FormatInt(item.ItemID, 10)
	}
	return result
}

// aggregateCandidates collapses chunk-level candidates into one candidate per
// knowledge item.  A synthetic description chunk can score higher than the
// actual file body for broad questions (for example, a category summary), so
// always use the best content chunk as the excerpt when content is available.
// Keep the maximum score across all chunks for channel ranking.
func aggregateCandidates(items []Candidate) []Candidate {
	type aggregate struct {
		selected Candidate
		best     float64
		content  *Candidate
	}

	entries := make(map[int64]*aggregate, len(items))
	order := make([]int64, 0, len(items))
	for _, item := range items {
		entry, ok := entries[item.ItemID]
		if !ok {
			copyItem := item
			entry = &aggregate{selected: copyItem, best: item.Score}
			entries[item.ItemID] = entry
			order = append(order, item.ItemID)
		} else if item.Score > entry.best {
			entry.best = item.Score
			entry.selected = item
		}
		if item.Score > entry.best {
			entry.best = item.Score
		}
		if strings.EqualFold(strings.TrimSpace(item.SourceKind), "content") {
			if entry.content == nil || item.Score > entry.content.Score {
				content := item
				entry.content = &content
			}
		}
	}

	result := make([]Candidate, 0, len(order))
	for _, itemID := range order {
		entry := entries[itemID]
		if entry == nil {
			continue
		}
		selected := entry.selected
		if entry.content != nil {
			selected = *entry.content
		}
		selected.Score = entry.best
		result = append(result, selected)
	}
	return result
}

func truncateRunes(value string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max]) + "…"
}

// ExtractFunc and OpenFunc keep infrastructure adapters small at composition
// time while preserving testable interfaces in this package.
type ExtractFunc func(context.Context, string, io.Reader) (string, error)

func (f ExtractFunc) ExtractText(ctx context.Context, filename string, source io.Reader) (string, error) {
	return f(ctx, filename, source)
}

type OpenFunc func(context.Context, string) (io.ReadCloser, error)

func (f OpenFunc) Open(ctx context.Context, fileKey string) (io.ReadCloser, error) {
	return f(ctx, fileKey)
}
