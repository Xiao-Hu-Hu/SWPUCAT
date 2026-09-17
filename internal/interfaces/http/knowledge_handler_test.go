package http

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	app "SWPUCAT/internal/application/knowledge"
	domain "SWPUCAT/internal/domain/knowledge"
	"SWPUCAT/internal/infrastructure/event"
	"SWPUCAT/internal/infrastructure/storage"

	"github.com/gin-gonic/gin"
)

type uploadRepository struct {
	domain.Repository
	item *domain.KnowledgeItem
	err  error
}

type lifecycleNotFoundRepository struct {
	domain.Repository
}

func (*lifecycleNotFoundRepository) FindItemByID(context.Context, int64) (*domain.KnowledgeItem, error) {
	return nil, domain.ErrItemNotFound
}

func (r *uploadRepository) CreateItem(_ context.Context, item *domain.KnowledgeItem) error {
	if r.err != nil {
		return r.err
	}
	item.ID = 1
	r.item = item
	return nil
}

func (r *uploadRepository) FindItemByID(context.Context, int64) (*domain.KnowledgeItem, error) {
	return r.item, nil
}

func uploadFixture(t *testing.T) (*KnowledgeHandler, *uploadRepository, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := storage.NewLocalStorage(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := &uploadRepository{}
	h := NewKnowledgeHandler(app.NewKnowledgeService(repo, event.NewNoOpPublisher()), store)
	return h, repo, dir
}

func TestKnowledgeLifecycleHandlersReturnNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler := NewKnowledgeHandler(
		app.NewKnowledgeService(&lifecycleNotFoundRepository{}, event.NewNoOpPublisher()),
		nil,
	)
	router := gin.New()
	withActor := func(next gin.HandlerFunc) gin.HandlerFunc {
		return func(c *gin.Context) {
			c.Set(ContextUserID, int64(1))
			c.Set(ContextUsername, "测试队长")
			next(c)
		}
	}
	router.POST("/items/:id/quarantine", withActor(handler.QuarantineItem))
	router.POST("/items/:id/restore", withActor(handler.RestoreItem))

	for _, path := range []string{"/items/999/quarantine", "/items/999/restore"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"reason":"复核不存在的条目"}`))
		request.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want %d: %s", path, recorder.Code, http.StatusNotFound, recorder.Body.String())
		}
	}
}

func multipartRequest(t *testing.T, fieldsFirst bool, category, description string, files int) *http.Request {
	t.Helper()
	var body bytes.Buffer
	w := multipart.NewWriter(&body)
	writeFields := func() {
		if err := w.WriteField("category_id", category); err != nil {
			t.Fatal(err)
		}
		if err := w.WriteField("description", description); err != nil {
			t.Fatal(err)
		}
	}
	if fieldsFirst {
		writeFields()
	}
	for i := 0; i < files; i++ {
		part, err := w.CreateFormFile("file", "资料.pdf")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.WriteString(part, "file content"); err != nil {
			t.Fatal(err)
		}
	}
	if !fieldsFirst {
		writeFields()
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/upload", &body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	return req
}

func uploadRouter(h *KnowledgeHandler) *gin.Engine {
	r := gin.New()
	r.POST("/upload", func(c *gin.Context) {
		c.Set(ContextUserID, int64(1))
		c.Set(ContextUsername, "测试用户")
		c.Set(ContextRole, "captain")
		h.UploadFile(c)
	})
	r.GET("/download/:id", h.DownloadFile)
	return r
}

func regularFiles(t *testing.T, dir string) []string {
	t.Helper()
	var paths []string
	if err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestUploadAndDownloadWithEitherFieldOrder(t *testing.T) {
	for _, fieldsFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "existing-file-first-client", true: "fields-first-client"}[fieldsFirst], func(t *testing.T) {
			h, repo, dir := uploadFixture(t)
			r := uploadRouter(h)
			res := httptest.NewRecorder()
			r.ServeHTTP(res, multipartRequest(t, fieldsFirst, "2", "中文说明", 1))
			if res.Code != http.StatusCreated {
				t.Fatalf("upload: %d %s", res.Code, res.Body.String())
			}
			if repo.item.CategoryID != 2 || repo.item.Description != "中文说明" || repo.item.Name != "资料.pdf" {
				t.Fatalf("metadata changed: %+v", repo.item)
			}
			if got := regularFiles(t, dir); len(got) != 1 {
				t.Fatalf("unexpected files: %v", got)
			}
			res = httptest.NewRecorder()
			r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/download/1", nil))
			if res.Code != http.StatusOK || res.Body.String() != "file content" {
				t.Fatalf("download: %d %q", res.Code, res.Body.String())
			}
		})
	}
}

func TestSafeDownloadFilename(t *testing.T) {
	for _, tc := range []struct {
		name, input, want string
	}{
		{name: "normal", input: "资料.pdf", want: "资料.pdf"},
		{name: "header controls and quote", input: "恶意\"\r\nX-Injected: yes.pdf", want: "恶意___X-Injected: yes.pdf"},
		{name: "path separators", input: `..\\secret/file.pdf`, want: "..__secret_file.pdf"},
		{name: "empty", input: " \r\n ", want: "download"},
		{name: "dot names", input: ".", want: "download"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := safeDownloadFilename(tc.input); got != tc.want {
				t.Fatalf("safeDownloadFilename(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDownloadFilenameHeaderIsSanitized(t *testing.T) {
	h, repo, _ := uploadFixture(t)
	r := uploadRouter(h)
	res := httptest.NewRecorder()
	r.ServeHTTP(res, multipartRequest(t, true, "2", "", 1))
	if res.Code != http.StatusCreated {
		t.Fatalf("upload: %d %s", res.Code, res.Body.String())
	}
	repo.item.Name = "恶意\"\r\nX-Injected: yes.pdf"

	res = httptest.NewRecorder()
	r.ServeHTTP(res, httptest.NewRequest(http.MethodGet, "/download/1", nil))
	if res.Code != http.StatusOK {
		t.Fatalf("download: %d %s", res.Code, res.Body.String())
	}
	header := res.Header().Get("Content-Disposition")
	if strings.ContainsAny(header, "\r\n") {
		t.Fatalf("content-disposition contains a control line break: %q", header)
	}
	if !strings.Contains(header, `filename="恶意___X-Injected: yes.pdf"`) {
		t.Fatalf("sanitized fallback filename missing from header: %q", header)
	}
}

func TestInvalidUploadsLeaveNoFilesOrRecords(t *testing.T) {
	for _, tc := range []struct {
		name, category, description string
		files                       int
	}{
		{"invalid-late-category", "invalid", "", 1},
		{"missing-category", "", "", 1},
		{"oversized-late-field", "2", strings.Repeat("x", maxKnowledgeMetadataFieldSize+1), 1},
		{"description-too-long", "2", strings.Repeat("中", maxKnowledgeDescriptionRunes+1), 1},
		{"duplicate-file", "2", "", 2},
		{"missing-file", "2", "", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, repo, dir := uploadFixture(t)
			res := httptest.NewRecorder()
			uploadRouter(h).ServeHTTP(res, multipartRequest(t, false, tc.category, tc.description, tc.files))
			if res.Code != http.StatusBadRequest {
				t.Fatalf("status %d", res.Code)
			}
			if repo.item != nil || len(regularFiles(t, dir)) != 0 {
				t.Fatal("invalid upload leaked data")
			}
		})
	}
}

func TestDatabaseFailurePreservesOldFileAndRemovesNewFile(t *testing.T) {
	h, repo, dir := uploadFixture(t)
	oldKey, _ := h.storage.Save("old.txt", strings.NewReader("keep"))
	repo.err = errors.New("database unavailable")
	res := httptest.NewRecorder()
	uploadRouter(h).ServeHTTP(res, multipartRequest(t, false, "2", "", 1))
	if res.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", res.Code)
	}
	got, _ := os.ReadFile(filepath.Join(dir, oldKey))
	if string(got) != "keep" || len(regularFiles(t, dir)) != 1 {
		t.Fatal("rollback changed existing storage")
	}
}

func TestReceiveUploadLimitAndTruncation(t *testing.T) {
	for _, name := range []string{"exact-limit", "over-limit", "truncated"} {
		t.Run(name, func(t *testing.T) {
			h, _, dir := uploadFixture(t)
			req := multipartRequest(t, false, "2", "", 1)
			limit := int64(len("file content"))
			if name == "over-limit" {
				limit--
			}
			if name == "truncated" {
				body, _ := io.ReadAll(req.Body)
				req.Body = io.NopCloser(bytes.NewReader(body[:len(body)-10]))
			}
			pending, _, err := h.receiveUpload(req, limit)
			if name == "exact-limit" {
				if err != nil {
					t.Fatal(err)
				}
				pending.Discard()
			} else if err == nil {
				pending.Discard()
				t.Fatal("invalid upload accepted")
			}
			if name == "over-limit" && !errors.Is(err, storage.ErrFileTooLarge) {
				t.Fatalf("wrong limit error: %v", err)
			}
			if len(regularFiles(t, dir)) != 0 {
				t.Fatal("temporary file leaked")
			}
		})
	}
}

// Generate the body incrementally to measure allocations independently of file size.
func BenchmarkStreamUpload64MiB(b *testing.B) {
	store, err := storage.NewLocalStorage(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	h := &KnowledgeHandler{storage: store}
	const size = 64 << 20
	b.SetBytes(size)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		pr, pw := io.Pipe()
		w := multipart.NewWriter(pw)
		go func() {
			part, err := w.CreateFormFile("file", "large.bin")
			if err == nil {
				_, err = io.CopyN(part, zeroReader{}, size)
			}
			if err == nil {
				err = w.WriteField("category_id", "2")
			}
			if err == nil {
				err = w.Close()
			}
			pw.CloseWithError(err)
		}()
		req := httptest.NewRequest(http.MethodPost, "/upload", pr)
		req.Header.Set("Content-Type", w.FormDataContentType())
		pending, _, err := h.receiveUpload(req, size)
		pr.Close()
		if err != nil {
			b.Fatal(err)
		}
		pending.Discard()
	}
}

type zeroReader struct{}

func (zeroReader) Read(buf []byte) (int, error) {
	clear(buf)
	return len(buf), nil
}
