package knowledgeindex

import (
	"errors"
	"strings"
)

var errChunkLimit = errors.New("knowledge chunk limit exceeded")

func splitText(value string, size, overlap int) []string {
	result, _ := splitTextBounded(value, size, overlap, 0)
	return result
}

func splitTextBounded(value string, size, overlap, max int) ([]string, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\r\n", "\n"))
	if value == "" || size <= 0 {
		return nil, nil
	}
	runes := []rune(value)
	if len(runes) <= size {
		return []string{value}, nil
	}
	if overlap < 0 {
		overlap = 0
	}
	if overlap >= size {
		overlap = size - 1
	}
	if overlap > size/2 {
		overlap = size / 2
	}
	result := make([]string, 0, (len(runes)+size-1)/size)
	for start := 0; start < len(runes); {
		end := start + size
		if end >= len(runes) {
			end = len(runes)
		} else {
			minimum := start + size/2
			if boundary := lastBoundary(runes, minimum, end); boundary > start {
				end = boundary
			}
		}
		chunk := strings.TrimSpace(string(runes[start:end]))
		if chunk != "" {
			if max > 0 && len(result) >= max {
				return nil, errChunkLimit
			}
			result = append(result, chunk)
		}
		if end == len(runes) {
			break
		}
		next := end - overlap
		if next <= start {
			next = start + 1
		}
		start = next
	}
	return result, nil
}

func lastBoundary(runes []rune, minimum, end int) int {
	for i := end; i > minimum; i-- {
		switch runes[i-1] {
		case '\n', '。', '！', '？', ';', '；', '.', '!', '?':
			return i
		}
	}
	return end
}
