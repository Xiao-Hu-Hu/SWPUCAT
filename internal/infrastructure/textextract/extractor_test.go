package textextract

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestExtractPlainTextStreamsAndLimitsInput(t *testing.T) {
	e := New(Limits{MaxInputBytes: 30, MaxTextBytes: 20})
	got, err := e.Extract(context.Background(), "说明.MD", strings.NewReader("  部署步骤  "))
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != FormatText || got.Text != "部署步骤" {
		t.Fatalf("got %#v", got)
	}
	_, err = New(Limits{MaxInputBytes: 5}).Extract(context.Background(), "large.txt", strings.NewReader("123456"))
	if !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("got %v", err)
	}
}

func TestExtractDOCX(t *testing.T) {
	file := makeZIP(t, map[string]string{
		"[Content_Types].xml": "<Types/>",
		"word/document.xml":   `<w:document xmlns:w="urn:w"><w:body><w:p><w:r><w:t>部署说明</w:t></w:r></w:p><w:p><w:r><w:t>数据库配置</w:t></w:r><w:tab/><w:r><w:t>端口 5432</w:t></w:r></w:p></w:body></w:document>`,
	})
	got, err := New(DefaultLimits()).Extract(context.Background(), "guide.docx", bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	if got.Format != FormatDOCX || got.Text != "部署说明\n数据库配置\t端口 5432" {
		t.Fatalf("got %q", got.Text)
	}
}

func TestExtractXLSXSharedAndInlineStrings(t *testing.T) {
	file := makeZIP(t, map[string]string{
		"xl/sharedStrings.xml":     `<sst><si><t>项目名称</t></si><si><t>数据库配置</t></si></sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c t="s"><v>0</v></c><c t="s"><v>1</v></c></row><row r="2"><c t="inlineStr"><is><t>PostgreSQL</t></is></c><c><v>5432</v></c></row></sheetData></worksheet>`,
	})
	got, err := New(DefaultLimits()).Extract(context.Background(), "表格.XLSX", bytes.NewReader(file))
	if err != nil {
		t.Fatal(err)
	}
	want := "项目名称\t数据库配置\nPostgreSQL\t5432"
	if got.Format != FormatXLSX || got.Text != want {
		t.Fatalf("got %q, want %q", got.Text, want)
	}
}

func TestExtractRejectsArchiveBombAndTooManyEntries(t *testing.T) {
	bomb := makeZIP(t, map[string]string{"word/document.xml": strings.Repeat("x", 2048)})
	_, err := New(Limits{MaxEntryBytes: 100, MaxArchiveBytes: 100, MaxCompressionRatio: 1000}).Extract(context.Background(), "bomb.docx", bytes.NewReader(bomb))
	if !errors.Is(err, ErrArchiveTooLarge) {
		t.Fatalf("got %v", err)
	}

	compressed := makeZIP(t, map[string]string{"word/document.xml": strings.Repeat("x", 2048)})
	_, err = New(Limits{MaxEntryBytes: 4096, MaxArchiveBytes: 4096, MaxCompressionRatio: 2}).Extract(context.Background(), "ratio.docx", bytes.NewReader(compressed))
	if !errors.Is(err, ErrSuspiciousArchive) {
		t.Fatalf("got %v", err)
	}

	entries := makeZIP(t, map[string]string{"word/document.xml": "<document/>", "a": "", "b": ""})
	_, err = New(Limits{MaxArchiveEntries: 2}).Extract(context.Background(), "many.docx", bytes.NewReader(entries))
	if !errors.Is(err, ErrTooManyArchiveEntries) {
		t.Fatalf("got %v", err)
	}
}

func TestExtractPDFHasExplicitUnavailableResult(t *testing.T) {
	_, err := New(DefaultLimits()).Extract(context.Background(), "manual.pdf", strings.NewReader("%PDF"))
	if !errors.Is(err, ErrPDFExtractionUnavailable) {
		t.Fatalf("got %v", err)
	}
}

func makeZIP(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for name, content := range entries {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
