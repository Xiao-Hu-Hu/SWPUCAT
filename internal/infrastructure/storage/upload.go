package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

var ErrFileTooLarge = errors.New("file exceeds upload limit")

// PendingFile stays private until the entire multipart request has been validated.
// Both paths are inside uploadDir, so publishing never copies the file again.
type PendingFile struct {
	Size    int64
	path    string
	fileKey string
	storage *LocalStorage
}

func (s *LocalStorage) Stage(ctx context.Context, filename string, reader io.Reader, maxSize int64) (*PendingFile, error) {
	tmpDir := filepath.Join(s.uploadDir, ".tmp")
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return nil, fmt.Errorf("create upload staging directory: %w", err)
	}
	now := time.Now()
	// The temporary name is random, while the published name keeps the existing
	// timestamp-plus-extension format used by LocalStorage.Save.
	tmp, err := os.CreateTemp(tmpDir, ".upload-*")
	if err != nil {
		return nil, fmt.Errorf("create upload staging file: %w", err)
	}
	pending := &PendingFile{
		path: tmp.Name(), fileKey: filepath.Join(now.Format("2006-01-02"), fmt.Sprintf("%d%s", now.UnixNano(), filepath.Ext(filename))), storage: s,
	}
	keep := false
	defer func() {
		tmp.Close()
		if !keep {
			pending.Discard()
		}
	}()

	// LimitReader reads one extra byte to distinguish an exact-limit file from an
	// oversized one. No multipart file-sized memory buffer is allocated.
	pending.Size, err = io.Copy(tmp, io.LimitReader(&uploadReader{ctx: ctx, reader: reader}, maxSize+1))
	if err != nil {
		return nil, err
	}
	if pending.Size > maxSize {
		return nil, ErrFileTooLarge
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := tmp.Close(); err != nil {
		return nil, fmt.Errorf("close upload staging file: %w", err)
	}
	keep = true
	return pending, nil
}

func (p *PendingFile) Commit(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	target := filepath.Join(p.storage.uploadDir, p.fileKey)
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return "", err
	}
	if _, err := os.Lstat(target); !os.IsNotExist(err) {
		return "", fmt.Errorf("upload destination unavailable: %s", p.fileKey)
	}
	if err := os.Rename(p.path, target); err != nil {
		return "", err
	}
	return p.fileKey, nil
}

func (p *PendingFile) Discard() error {
	err := os.Remove(p.path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

type uploadReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r *uploadReader) Read(buf []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buf)
}
