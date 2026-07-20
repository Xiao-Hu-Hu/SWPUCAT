package knowledge

import (
	"errors"
	"path/filepath"
	"strings"
)

type ItemType string

const (
	ItemTypeLink ItemType = "link"
	ItemTypeFile ItemType = "file"
)

func (t ItemType) IsValid() bool {
	return t == ItemTypeLink || t == ItemTypeFile
}

var allowedFileTypes = map[string]bool{
	".pdf":  true,
	".doc":  true,
	".docx": true,
	".md":   true,
	".zip":  true,
	".exe":  true,
	".txt":  true,
	".ppt":  true,
	".pptx": true,
	".xls":  true,
	".xlsx": true,
	".rar":  true,
	".7z":   true,
}

func IsAllowedFileType(filename string) bool {
	ext := strings.ToLower(filepath.Ext(filename))
	return allowedFileTypes[ext]
}

var (
	ErrItemNotFound      = errors.New("knowledge item not found")
	ErrCategoryNotFound  = errors.New("category not found")
	ErrEmptyItemName     = errors.New("item name cannot be empty")
	ErrEmptyURL          = errors.New("link URL cannot be empty")
	ErrCategoryInUse     = errors.New("category is in use, cannot delete")
	ErrFileTypeNotAllowed = errors.New("file type not allowed")
)
