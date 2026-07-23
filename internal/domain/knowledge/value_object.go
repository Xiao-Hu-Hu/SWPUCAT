package knowledge

import (
	"errors"
)

type ItemType string

const (
	ItemTypeLink ItemType = "link"
	ItemTypeFile ItemType = "file"
)

func (t ItemType) IsValid() bool {
	return t == ItemTypeLink || t == ItemTypeFile
}

func IsAllowedFileType(filename string) bool {
	// 允许所有文件类型
	return true
}

var (
	ErrItemNotFound      = errors.New("knowledge item not found")
	ErrCategoryNotFound  = errors.New("category not found")
	ErrEmptyItemName     = errors.New("item name cannot be empty")
	ErrEmptyURL          = errors.New("link URL cannot be empty")
	ErrCategoryInUse     = errors.New("category is in use, cannot delete")
	ErrFileTypeNotAllowed = errors.New("file type not allowed")
)
