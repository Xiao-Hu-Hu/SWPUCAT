package knowledgeindex

import (
	"context"
	"errors"
	"io"
	"time"
)

var (
	ErrNoPendingJob = errors.New("no pending knowledge index job")
	ErrStaleJob     = errors.New("knowledge index job is stale")
	ErrInvalidQuery = errors.New("knowledge search query is invalid")
)

// Source is the approved knowledge projection needed by the indexer. FileKey
// remains inside the server process and is never returned from search APIs.
type Source struct {
	ID           int64
	Type         string
	Name         string
	Description  string
	URL          string
	FileKey      string
	CategoryName string
}

type Job struct {
	Source             Source
	ClaimedFingerprint string
	Attempts           int
}

type Chunk struct {
	SourceKind          string
	Index               int
	Content             string
	ContentHash         string
	Embedding           []float32
	EmbeddingModel      string
	EmbeddingDimensions int
	MetadataJSON        string
}

type Candidate struct {
	ChunkID      int64
	ItemID       int64
	Type         string
	Name         string
	Description  string
	URL          string
	CategoryName string
	SourceKind   string
	Excerpt      string
	Score        float64
}

type SearchResult struct {
	ItemID       int64   `json:"id"`
	Type         string  `json:"type"`
	Name         string  `json:"name"`
	Description  string  `json:"description,omitempty"`
	URL          string  `json:"url,omitempty"`
	DownloadURL  string  `json:"download_url,omitempty"`
	CategoryName string  `json:"category_name"`
	SourceKind   string  `json:"source_kind"`
	Excerpt      string  `json:"excerpt"`
	Score        float64 `json:"score"`
	MatchMode    string  `json:"match_mode"`
}

type Repository interface {
	ListApprovedSources(ctx context.Context) ([]Source, error)
	EnsureJob(ctx context.Context, itemID int64, fingerprint string) error
	ClaimJob(ctx context.Context, now, staleBefore time.Time, maxAttempts int) (*Job, error)
	CompleteJob(ctx context.Context, job *Job, finalFingerprint string, chunks []Chunk) error
	CompletePartialJob(ctx context.Context, job *Job, sourceFingerprint string, chunks []Chunk, message string, retryAt time.Time, maxAttempts int) error
	FailJob(ctx context.Context, job *Job, message string, retryAt time.Time, maxAttempts int) error
	KeywordSearch(ctx context.Context, query string, limit int) ([]Candidate, error)
	VectorSearch(ctx context.Context, vector []float32, model string, dimensions, limit int, minScore float64) ([]Candidate, error)
}

type Embedder interface {
	Embed(ctx context.Context, input string) ([]float32, error)
	EmbedBatch(ctx context.Context, inputs []string) ([][]float32, error)
}

type TextExtractor interface {
	ExtractText(ctx context.Context, filename string, source io.Reader) (string, error)
}

type FileStore interface {
	Open(ctx context.Context, fileKey string) (io.ReadCloser, error)
}
