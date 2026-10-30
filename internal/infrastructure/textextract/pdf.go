package textextract

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	pdfreader "github.com/ledongthuc/pdf"
)

func extractPDF(ctx context.Context, data []byte, limits Limits) (text string, err error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(data) < len("%PDF-") || !bytes.HasPrefix(data, []byte("%PDF-")) {
		return "", ErrMalformedPDF
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			text = ""
			err = fmt.Errorf("parse pdf: %w", ErrMalformedPDF)
		}
	}()

	document, openErr := pdfreader.NewReader(bytes.NewReader(data), int64(len(data)))
	if openErr != nil {
		return "", fmt.Errorf("open pdf: %w", ErrMalformedPDF)
	}
	pages := document.NumPage()
	if pages < 0 {
		return "", ErrMalformedPDF
	}
	if pages > limits.MaxPDFPages {
		return "", fmt.Errorf("%w: %d > %d", ErrTooManyPDFPages, pages, limits.MaxPDFPages)
	}

	out := newTextBuffer(limits.MaxTextBytes)
	wrotePage := false
	for pageNumber := 1; pageNumber <= pages; pageNumber++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		pageText, pageErr := document.Page(pageNumber).GetPlainText(nil)
		if pageErr != nil {
			if errors.Is(pageErr, context.Canceled) || errors.Is(pageErr, context.DeadlineExceeded) {
				return "", pageErr
			}
			return "", fmt.Errorf("extract pdf page %d: %w", pageNumber, ErrMalformedPDF)
		}
		pageText = strings.TrimSpace(pageText)
		if pageText == "" {
			continue
		}
		if wrotePage {
			if err := out.WriteString("\n\n"); err != nil {
				return "", err
			}
		}
		if err := out.WriteString(pageText); err != nil {
			return "", err
		}
		wrotePage = true
	}
	return out.String(), nil
}
