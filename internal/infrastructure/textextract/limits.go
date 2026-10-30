package textextract

import (
	"context"
	"io"
	"strings"
)

type textBuffer struct {
	b   strings.Builder
	max int64
	n   int64
}

func newTextBuffer(max int64) *textBuffer { return &textBuffer{max: max} }

func (b *textBuffer) WriteString(s string) error {
	if int64(len(s)) > b.max-b.n {
		return ErrTextTooLarge
	}
	b.b.WriteString(s)
	b.n += int64(len(s))
	return nil
}

func (b *textBuffer) String() string { return b.b.String() }

type limitedString struct {
	b   strings.Builder
	max int64
}

func newLimitedString(max int64) *limitedString { return &limitedString{max: max} }
func (b *limitedString) Reset()                 { b.b.Reset() }
func (b *limitedString) WriteString(s string) error {
	if int64(len(s)) > b.max-int64(b.b.Len()) {
		return ErrTextTooLarge
	}
	b.b.WriteString(s)
	return nil
}
func (b *limitedString) String() string { return b.b.String() }

// hardLimitReader checks the context and probes after the configured limit.
// The probe prevents a forged ZIP central-directory size from bypassing the
// decompression budget.
type hardLimitReader struct {
	ctx       context.Context
	r         io.Reader
	remaining int64
	budget    *archiveBudget
}

type archiveBudget struct{ remaining int64 }

func (r *hardLimitReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	available := r.remaining
	if r.budget != nil && r.budget.remaining < available {
		available = r.budget.remaining
	}
	if available > 0 {
		if int64(len(p)) > available {
			p = p[:available]
		}
		n, err := r.r.Read(p)
		r.remaining -= int64(n)
		if r.budget != nil {
			r.budget.remaining -= int64(n)
		}
		return n, err
	}
	var probe [1]byte
	n, err := r.r.Read(probe[:])
	if n > 0 {
		return 0, ErrArchiveTooLarge
	}
	return 0, err
}
