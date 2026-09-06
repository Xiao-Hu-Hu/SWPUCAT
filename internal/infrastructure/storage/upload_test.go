package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStagedUploadKeepsExistingFilesReadable(t *testing.T) {
	s, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	oldKey, err := s.Save("旧文件.pdf", strings.NewReader("existing content"))
	if err != nil {
		t.Fatal(err)
	}
	pending, err := s.Stage(context.Background(), "新文件.pdf", strings.NewReader("new content"), 11)
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Discard()
	if _, err := s.Get(pending.fileKey); err == nil {
		t.Fatal("uncommitted upload was visible at its final path")
	}
	key, err := pending.Commit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for fileKey, want := range map[string]string{oldKey: "existing content", key: "new content"} {
		path, err := s.Get(fileKey)
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("file %s: content=%q err=%v", fileKey, got, err)
		}
	}
	if filepath.Ext(key) != ".pdf" || filepath.Dir(key) != filepath.Dir(oldKey) {
		t.Fatalf("unexpected storage layout: %s", key)
	}
	if _, err := os.Stat(pending.path); !os.IsNotExist(err) {
		t.Fatal("temporary file still exists after commit")
	}
}

func TestStageCleansUpFailedUploads(t *testing.T) {
	for _, name := range []string{"oversized", "interrupted", "cancelled"} {
		t.Run(name, func(t *testing.T) {
			s, _ := NewLocalStorage(t.TempDir())
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var reader io.Reader = strings.NewReader("too large")
			want := ErrFileTooLarge
			if name == "interrupted" {
				reader = io.MultiReader(strings.NewReader("ok"), failingReader{})
				want = io.ErrUnexpectedEOF
			} else if name == "cancelled" {
				reader = &cancellingReader{cancel: cancel}
				want = context.Canceled
			}
			if _, err := s.Stage(ctx, "file.bin", reader, 4); !errors.Is(err, want) {
				t.Fatalf("got %v, want %v", err, want)
			}
			entries, err := os.ReadDir(filepath.Join(s.uploadDir, ".tmp"))
			if err != nil || len(entries) != 0 {
				t.Fatalf("staging files leaked: %v, %v", entries, err)
			}
		})
	}
}

func TestCommitDoesNotOverwriteExistingFile(t *testing.T) {
	s, _ := NewLocalStorage(t.TempDir())
	key, _ := s.Save("old.bin", strings.NewReader("keep"))
	pending, err := s.Stage(context.Background(), "new.bin", strings.NewReader("new"), 3)
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Discard()
	pending.fileKey = key
	if _, err := pending.Commit(context.Background()); err == nil {
		t.Fatal("commit accepted an existing destination")
	}
	got, _ := os.ReadFile(filepath.Join(s.uploadDir, key))
	if string(got) != "keep" {
		t.Fatal("existing file was changed")
	}
}

type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }

type cancellingReader struct{ cancel context.CancelFunc }

func (r *cancellingReader) Read(buf []byte) (int, error) {
	buf[0] = 'x'
	r.cancel()
	return 1, nil
}
