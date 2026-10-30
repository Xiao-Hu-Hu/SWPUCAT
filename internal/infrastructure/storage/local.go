package storage

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var ErrInvalidFileKey = errors.New("invalid file key")

type LocalStorage struct {
	uploadDir string
}

func NewLocalStorage(uploadDir string) (*LocalStorage, error) {
	absUploadDir, err := filepath.Abs(uploadDir)
	if err != nil {
		return nil, fmt.Errorf("resolve upload directory: %w", err)
	}
	if err := os.MkdirAll(absUploadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}
	return &LocalStorage{uploadDir: absUploadDir}, nil
}

func (s *LocalStorage) Save(filename string, reader io.Reader) (string, error) {
	// Generate unique file key
	ext := filepath.Ext(filename)
	fileKey := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)

	// Create subdirectory by date
	dateDir := time.Now().Format("2006-01-02")
	dirPath := filepath.Join(s.uploadDir, dateDir)
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", fmt.Errorf("failed to create date directory: %w", err)
	}

	filePath := filepath.Join(dirPath, fileKey)
	dst, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, reader); err != nil {
		os.Remove(filePath)
		return "", fmt.Errorf("failed to save file: %w", err)
	}

	return filepath.Join(dateDir, fileKey), nil
}

func (s *LocalStorage) Get(fileKey string) (string, error) {
	filePath, err := s.safePath(fileKey)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return "", fmt.Errorf("file not found")
	}
	return filePath, nil
}

func (s *LocalStorage) Delete(fileKey string) error {
	filePath, err := s.safePath(fileKey)
	if err != nil {
		return err
	}
	return os.Remove(filePath)
}

func (s *LocalStorage) safePath(fileKey string) (string, error) {
	normalizedKey := filepath.FromSlash(strings.ReplaceAll(fileKey, `\`, "/"))
	if normalizedKey == "" || filepath.IsAbs(normalizedKey) {
		return "", ErrInvalidFileKey
	}
	cleanKey := filepath.Clean(normalizedKey)
	if cleanKey == "." || cleanKey == ".." || strings.HasPrefix(cleanKey, ".."+string(os.PathSeparator)) {
		return "", ErrInvalidFileKey
	}
	filePath := filepath.Join(s.uploadDir, cleanKey)
	relative, err := filepath.Rel(s.uploadDir, filePath)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(os.PathSeparator)) {
		return "", ErrInvalidFileKey
	}
	return filePath, nil
}
