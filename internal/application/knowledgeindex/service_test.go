package knowledgeindex

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	sources     []Source
	job         *Job
	ensured     map[int64]string
	completed   []Chunk
	completedFP string
	failed      string
	partial     string
	keyword     []Candidate
	vector      []Candidate
	keywordErr  error
	vectorErr   error
}

func (r *fakeRepository) ListApprovedSources(context.Context) ([]Source, error) {
	return append([]Source(nil), r.sources...), nil
}
func (r *fakeRepository) EnsureJob(_ context.Context, id int64, fingerprint string) error {
	if r.ensured == nil {
		r.ensured = map[int64]string{}
	}
	r.ensured[id] = fingerprint
	return nil
}
func (r *fakeRepository) ClaimJob(context.Context, time.Time, time.Time, int) (*Job, error) {
	if r.job == nil {
		return nil, ErrNoPendingJob
	}
	job := r.job
	r.job = nil
	return job, nil
}
func (r *fakeRepository) CompleteJob(_ context.Context, _ *Job, fingerprint string, chunks []Chunk) error {
	r.completedFP = fingerprint
	r.completed = append([]Chunk(nil), chunks...)
	return nil
}
func (r *fakeRepository) CompletePartialJob(_ context.Context, _ *Job, fingerprint string, chunks []Chunk, message string, _ time.Time, _ int) error {
	r.completedFP = fingerprint
	r.completed = append([]Chunk(nil), chunks...)
	r.partial = message
	return nil
}
func (r *fakeRepository) FailJob(_ context.Context, _ *Job, message string, _ time.Time, _ int) error {
	r.failed = message
	return nil
}
func (r *fakeRepository) KeywordSearch(context.Context, string, int) ([]Candidate, error) {
	return r.keyword, r.keywordErr
}
func (r *fakeRepository) VectorSearch(context.Context, []float32, string, int, int, float64) ([]Candidate, error) {
	return r.vector, r.vectorErr
}

type fakeEmbedder struct {
	vector []float32
	err    error
}

func (e fakeEmbedder) Embed(context.Context, string) ([]float32, error) {
	return append([]float32(nil), e.vector...), e.err
}
func (e fakeEmbedder) EmbedBatch(_ context.Context, values []string) ([][]float32, error) {
	if e.err != nil {
		return nil, e.err
	}
	result := make([][]float32, len(values))
	for i := range result {
		result[i] = append([]float32(nil), e.vector...)
	}
	return result, nil
}

func testConfig(mode string) Config {
	return Config{Mode: mode, CandidateLimit: 10, FinalLimit: 5, MinVectorScore: 0.2, ChunkRunes: 40, ChunkOverlapRunes: 5, MaxChunks: 100, MaxExtractedRunes: 1000, WorkerInterval: time.Second, EmbeddingModel: "embed-test", EmbeddingDimensions: 2}
}

func TestSplitTextBoundsAndOverlap(t *testing.T) {
	text := "第一段内容比较长，需要在句号处分割。\n\n第二段继续描述知识库的具体使用步骤，并保留少量上下文。"
	chunks := splitText(text, 24, 5)
	if len(chunks) < 2 {
		t.Fatalf("chunks = %#v", chunks)
	}
	for _, chunk := range chunks {
		if len([]rune(chunk)) > 24 {
			t.Fatalf("chunk exceeds limit: %q", chunk)
		}
	}
	if !strings.Contains(chunks[0], "第一段") || !strings.Contains(strings.Join(chunks, ""), "第二段") {
		t.Fatalf("unexpected chunks: %#v", chunks)
	}
}

func TestRunOnceIndexesDescriptionAndFileContent(t *testing.T) {
	repo := &fakeRepository{job: &Job{Source: Source{ID: 7, Type: "file", Name: "说明.docx", FileKey: "2026/file.docx", CategoryName: "制度", Description: "用于查询请假流程"}, Attempts: 1}}
	service, err := NewService(
		repo,
		fakeEmbedder{vector: []float32{0.2, 0.8}},
		ExtractFunc(func(_ context.Context, filename string, source io.Reader) (string, error) {
			body, _ := io.ReadAll(source)
			return filename + ":" + string(body), nil
		}),
		OpenFunc(func(context.Context, string) (io.ReadCloser, error) {
			return io.NopCloser(strings.NewReader("正文里的审批步骤")), nil
		}),
		testConfig("hybrid"),
	)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.RunOnce(context.Background())
	if err != nil || !claimed {
		t.Fatalf("RunOnce() = %v, %v", claimed, err)
	}
	if len(repo.completed) < 2 || repo.completedFP == "" {
		t.Fatalf("completed chunks = %#v, fp = %q", repo.completed, repo.completedFP)
	}
	kinds := map[string]bool{}
	for _, chunk := range repo.completed {
		kinds[chunk.SourceKind] = true
		if len(chunk.Embedding) != 2 || chunk.EmbeddingModel != "embed-test" {
			t.Fatalf("missing embedding: %#v", chunk)
		}
	}
	if !kinds["description"] || !kinds["content"] {
		t.Fatalf("source kinds = %#v", kinds)
	}
}

func TestRunOnceKeepsDescriptionSearchableOnExtractionFailure(t *testing.T) {
	repo := &fakeRepository{job: &Job{Source: Source{ID: 8, Type: "file", Name: "bad.pdf", FileKey: "bad.pdf"}, Attempts: 2}}
	service, err := NewService(repo, nil, ExtractFunc(func(context.Context, string, io.Reader) (string, error) {
		return "", errors.New("broken pdf")
	}), OpenFunc(func(context.Context, string) (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("bad")), nil
	}), testConfig("keyword"))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.RunOnce(context.Background())
	if !claimed || err == nil || len(repo.completed) == 0 || repo.completed[0].SourceKind != "description" || !strings.Contains(repo.partial, "broken pdf") {
		t.Fatalf("RunOnce() = %v, %v, completed=%#v, partial=%q, failed=%q", claimed, err, repo.completed, repo.partial, repo.failed)
	}
}

func TestKeywordIndexDoesNotDependOnConfiguredEmbedder(t *testing.T) {
	repo := &fakeRepository{job: &Job{Source: Source{ID: 9, Type: "link", Name: "考勤制度", Description: "每周打卡要求"}, Attempts: 1}}
	service, err := NewService(repo, fakeEmbedder{err: errors.New("embedding unavailable")}, nil, nil, testConfig("keyword"))
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := service.RunOnce(context.Background())
	if !claimed || err != nil || len(repo.completed) == 0 {
		t.Fatalf("RunOnce() = %v, %v, completed=%#v", claimed, err, repo.completed)
	}
}

func TestReconcileFingerprintChangesWithIndexConfiguration(t *testing.T) {
	source := Source{ID: 1, Type: "link", Name: "部署指南", URL: "https://example.test", Description: "运行方式"}
	repoA := &fakeRepository{sources: []Source{source}}
	serviceA, _ := NewService(repoA, nil, nil, nil, testConfig("keyword"))
	if _, err := serviceA.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	repoB := &fakeRepository{sources: []Source{source}}
	configB := testConfig("keyword")
	configB.ChunkRunes++
	serviceB, _ := NewService(repoB, nil, nil, nil, configB)
	_, _ = serviceB.Reconcile(context.Background())
	if repoA.ensured[1] == "" || repoA.ensured[1] == repoB.ensured[1] {
		t.Fatalf("fingerprints did not reflect configuration: %#v %#v", repoA.ensured, repoB.ensured)
	}
	repoC := &fakeRepository{sources: []Source{source}}
	configC := testConfig("vector")
	configC.MaxChunks = 101
	serviceC, _ := NewService(repoC, fakeEmbedder{vector: []float32{1, 0}}, nil, nil, configC)
	_, _ = serviceC.Reconcile(context.Background())
	if repoA.ensured[1] == repoC.ensured[1] {
		t.Fatalf("fingerprint did not reflect retrieval mode/chunk cap: %#v %#v", repoA.ensured, repoC.ensured)
	}
}

func TestHybridSearchFusesAndFallsBackToKeyword(t *testing.T) {
	repo := &fakeRepository{
		vector:  []Candidate{{ItemID: 2, Name: "向量第一", Score: 0.9}, {ItemID: 1, Name: "共同结果", Score: 0.8}},
		keyword: []Candidate{{ItemID: 1, Name: "共同结果", Score: 0.7}, {ItemID: 3, Name: "关键词第二", Score: 0.6}},
	}
	service, err := NewService(repo, fakeEmbedder{vector: []float32{1, 0}}, nil, nil, testConfig("hybrid"))
	if err != nil {
		t.Fatal(err)
	}
	results, err := service.Search(context.Background(), "怎么部署")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || results[0].ItemID != 1 || results[0].MatchMode != "hybrid" {
		t.Fatalf("results = %#v", results)
	}

	service.embedder = fakeEmbedder{err: errors.New("embedding unavailable")}
	results, err = service.Search(context.Background(), "怎么部署")
	if err != nil || len(results) != 2 || results[0].MatchMode != "keyword" {
		t.Fatalf("fallback = %#v, %v", results, err)
	}
}

func TestSearchRejectsInvalidQuery(t *testing.T) {
	service, _ := NewService(&fakeRepository{}, nil, nil, nil, testConfig("keyword"))
	if _, err := service.Search(context.Background(), strings.Repeat("问", maxQueryRunes+1)); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("error = %v", err)
	}
}
