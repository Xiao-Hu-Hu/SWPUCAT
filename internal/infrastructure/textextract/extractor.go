// Package textextract extracts indexable text from knowledge-base attachments.
//
// Office formats are read directly from their ZIP/XML structure and PDF files
// are parsed in process by a pure-Go reader, so no office suite or external
// command is required in the server image.
package textextract

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

var (
	ErrUnsupportedFormat     = errors.New("unsupported document format")
	ErrInputTooLarge         = errors.New("input exceeds extraction limit")
	ErrArchiveTooLarge       = errors.New("archive exceeds extraction limit")
	ErrTooManyArchiveEntries = errors.New("archive has too many entries")
	ErrSuspiciousArchive     = errors.New("archive compression ratio exceeds limit")
	ErrTextTooLarge          = errors.New("extracted text exceeds limit")
	ErrMalformedDocument     = errors.New("malformed office document")
	ErrMalformedPDF          = errors.New("malformed or unsupported pdf document")
	ErrTooManyPDFPages       = errors.New("pdf has too many pages")
)

// Format identifies the source format used to produce Result.Text.
type Format string

const (
	FormatText Format = "text"
	FormatDOCX Format = "docx"
	FormatXLSX Format = "xlsx"
	FormatPDF  Format = "pdf"
)

// Limits bound both memory use and decompression work.  Zero fields receive
// conservative defaults; callers may tighten individual fields for a job.
type Limits struct {
	MaxInputBytes        int64
	MaxArchiveEntries    int
	MaxEntryBytes        int64
	MaxArchiveBytes      int64
	MaxTextBytes         int64
	MaxSharedStringBytes int64
	MaxCompressionRatio  int64
	MaxPDFPages          int
}

func DefaultLimits() Limits {
	return Limits{
		MaxInputBytes:        16 << 20,
		MaxArchiveEntries:    256,
		MaxEntryBytes:        8 << 20,
		MaxArchiveBytes:      12 << 20,
		MaxTextBytes:         2 << 20,
		MaxSharedStringBytes: 2 << 20,
		MaxCompressionRatio:  100,
		MaxPDFPages:          200,
	}
}

func (l Limits) withDefaults() Limits {
	d := DefaultLimits()
	if l.MaxInputBytes <= 0 {
		l.MaxInputBytes = d.MaxInputBytes
	}
	if l.MaxArchiveEntries <= 0 {
		l.MaxArchiveEntries = d.MaxArchiveEntries
	}
	if l.MaxEntryBytes <= 0 {
		l.MaxEntryBytes = d.MaxEntryBytes
	}
	if l.MaxArchiveBytes <= 0 {
		l.MaxArchiveBytes = d.MaxArchiveBytes
	}
	if l.MaxTextBytes <= 0 {
		l.MaxTextBytes = d.MaxTextBytes
	}
	if l.MaxSharedStringBytes <= 0 {
		l.MaxSharedStringBytes = d.MaxSharedStringBytes
	}
	if l.MaxCompressionRatio <= 0 {
		l.MaxCompressionRatio = d.MaxCompressionRatio
	}
	if l.MaxPDFPages <= 0 {
		l.MaxPDFPages = d.MaxPDFPages
	}
	return l
}

// Result is safe to persist with an index job.  It contains text only; callers
// keep source metadata and authorization outside this package.
type Result struct {
	Format Format
	Text   string
}

// Extractor is safe to reuse concurrently.  It does not retain source bytes.
type Extractor struct{ limits Limits }

func New(limits Limits) *Extractor { return &Extractor{limits: limits.withDefaults()} }

// Extract reads one attachment. Text formats are copied incrementally. ZIP
// formats require a bounded in-memory copy because archive/zip needs ReaderAt;
// every copy and every decompressed entry is limited before XML is parsed.
func (e *Extractor) Extract(ctx context.Context, filename string, source io.Reader) (Result, error) {
	if source == nil {
		return Result{}, fmt.Errorf("extract %q: nil source", filename)
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	ext := strings.ToLower(filepath.Ext(filename))
	switch ext {
	case ".md", ".markdown", ".txt":
		text, err := readPlain(ctx, source, e.limits)
		if err != nil {
			return Result{}, fmt.Errorf("extract text: %w", err)
		}
		return Result{Format: FormatText, Text: strings.TrimSpace(text)}, nil
	case ".docx", ".xlsx":
		archive, err := readArchive(ctx, source, e.limits.MaxInputBytes)
		if err != nil {
			return Result{}, fmt.Errorf("read office archive: %w", err)
		}
		zr, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
		if err != nil {
			return Result{}, fmt.Errorf("open office archive: %w", ErrMalformedDocument)
		}
		if err := validateArchive(zr, e.limits); err != nil {
			return Result{}, err
		}
		var text string
		if ext == ".docx" {
			text, err = extractDOCX(ctx, zr, e.limits)
		} else {
			text, err = extractXLSX(ctx, zr, e.limits)
		}
		if err != nil {
			return Result{}, err
		}
		format := FormatDOCX
		if ext == ".xlsx" {
			format = FormatXLSX
		}
		return Result{Format: format, Text: strings.TrimSpace(text)}, nil
	case ".pdf":
		data, err := readArchive(ctx, source, e.limits.MaxInputBytes)
		if err != nil {
			return Result{}, fmt.Errorf("read pdf: %w", err)
		}
		text, err := extractPDF(ctx, data, e.limits)
		if err != nil {
			return Result{}, err
		}
		return Result{Format: FormatPDF, Text: strings.TrimSpace(text)}, nil
	default:
		return Result{}, fmt.Errorf("%w: %s", ErrUnsupportedFormat, ext)
	}
}

func readPlain(ctx context.Context, source io.Reader, limits Limits) (string, error) {
	b := newTextBuffer(limits.MaxTextBytes)
	buf := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		n, err := source.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > limits.MaxInputBytes {
				return "", ErrInputTooLarge
			}
			if err := b.WriteString(string(buf[:n])); err != nil {
				return "", err
			}
		}
		if err == io.EOF {
			return b.String(), nil
		}
		if err != nil {
			return "", err
		}
	}
}

func readArchive(ctx context.Context, source io.Reader, max int64) ([]byte, error) {
	var b bytes.Buffer
	buf := make([]byte, 32*1024)
	var total int64
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		n, err := source.Read(buf)
		if n > 0 {
			total += int64(n)
			if total > max {
				return nil, ErrInputTooLarge
			}
			_, _ = b.Write(buf[:n])
		}
		if err == io.EOF {
			return b.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func validateArchive(zr *zip.Reader, limits Limits) error {
	if len(zr.File) > limits.MaxArchiveEntries {
		return ErrTooManyArchiveEntries
	}
	var declared uint64
	for _, f := range zr.File {
		if f.UncompressedSize64 > uint64(limits.MaxEntryBytes) {
			return ErrArchiveTooLarge
		}
		if f.UncompressedSize64 > 0 {
			if f.CompressedSize64 == 0 || (f.CompressedSize64 <= ^uint64(0)/uint64(limits.MaxCompressionRatio) && f.UncompressedSize64 > f.CompressedSize64*uint64(limits.MaxCompressionRatio)) {
				return ErrSuspiciousArchive
			}
		}
		if ^uint64(0)-declared < f.UncompressedSize64 {
			return ErrArchiveTooLarge
		}
		declared += f.UncompressedSize64
		if declared > uint64(limits.MaxArchiveBytes) {
			return ErrArchiveTooLarge
		}
	}
	return nil
}
