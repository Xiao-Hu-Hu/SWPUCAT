package textextract

import (
	"archive/zip"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

func extractDOCX(ctx context.Context, zr *zip.Reader, limits Limits) (string, error) {
	f := findFile(zr, "word/document.xml")
	if f == nil {
		return "", ErrMalformedDocument
	}
	b := newTextBuffer(limits.MaxTextBytes)
	budget := &archiveBudget{remaining: limits.MaxArchiveBytes}
	dec, closeEntry, err := openXML(ctx, f, limits, budget)
	if err != nil {
		return "", err
	}
	defer closeEntry()
	insideText := 0
	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		tok, err := dec.Token()
		if err == io.EOF {
			return b.String(), nil
		}
		if err != nil {
			return "", documentError("parse docx XML", err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			switch v.Name.Local {
			case "t":
				insideText++
			case "tab":
				if err := b.WriteString("\t"); err != nil {
					return "", err
				}
			case "br", "cr":
				if err := b.WriteString("\n"); err != nil {
					return "", err
				}
			}
		case xml.CharData:
			if insideText > 0 {
				if err := b.WriteString(string(v)); err != nil {
					return "", err
				}
			}
		case xml.EndElement:
			if v.Name.Local == "t" && insideText > 0 {
				insideText--
			}
			if v.Name.Local == "p" {
				if err := b.WriteString("\n"); err != nil {
					return "", err
				}
			}
		}
	}
}

func extractXLSX(ctx context.Context, zr *zip.Reader, limits Limits) (string, error) {
	budget := &archiveBudget{remaining: limits.MaxArchiveBytes}
	var shared []string
	if f := findFile(zr, "xl/sharedStrings.xml"); f != nil {
		var err error
		shared, err = readSharedStrings(ctx, f, limits, budget)
		if err != nil {
			return "", err
		}
	}
	files := make([]*zip.File, 0)
	for _, f := range zr.File {
		if isWorksheet(f.Name) {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		return "", ErrMalformedDocument
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	b := newTextBuffer(limits.MaxTextBytes)
	for _, f := range files {
		if err := readWorksheet(ctx, f, shared, limits, budget, b); err != nil {
			return "", err
		}
	}
	return b.String(), nil
}

func readSharedStrings(ctx context.Context, f *zip.File, limits Limits, budget *archiveBudget) ([]string, error) {
	dec, closeEntry, err := openXML(ctx, f, limits, budget)
	if err != nil {
		return nil, err
	}
	defer closeEntry()
	var values []string
	var item strings.Builder
	insideItem, insideText := false, 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		tok, err := dec.Token()
		if err == io.EOF {
			return values, nil
		}
		if err != nil {
			return nil, documentError("parse xlsx shared strings", err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			if v.Name.Local == "si" {
				insideItem = true
				item.Reset()
			}
			if insideItem && v.Name.Local == "t" {
				insideText++
			}
		case xml.CharData:
			if insideText > 0 {
				if item.Len()+len(v) > int(limits.MaxSharedStringBytes) {
					return nil, ErrTextTooLarge
				}
				item.Write([]byte(v))
			}
		case xml.EndElement:
			if v.Name.Local == "t" && insideText > 0 {
				insideText--
			}
			if v.Name.Local == "si" && insideItem {
				values = append(values, item.String())
				insideItem = false
			}
		}
	}
}

func readWorksheet(ctx context.Context, f *zip.File, shared []string, limits Limits, budget *archiveBudget, out *textBuffer) error {
	dec, closeEntry, err := openXML(ctx, f, limits, budget)
	if err != nil {
		return err
	}
	defer closeEntry()
	inRow, firstCell, inCell := false, true, false
	cellType, value, inline := "", newLimitedString(limits.MaxSharedStringBytes), newLimitedString(limits.MaxSharedStringBytes)
	inValue, inText := false, 0
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		tok, err := dec.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return documentError("parse xlsx worksheet", err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			switch v.Name.Local {
			case "row":
				inRow = true
				firstCell = true
			case "c":
				inCell = true
				cellType = attr(v.Attr, "t")
				value.Reset()
				inline.Reset()
			case "v":
				if inCell {
					inValue = true
				}
			case "t":
				if inCell {
					inText++
				}
			}
		case xml.CharData:
			if inValue {
				if err := value.WriteString(string(v)); err != nil {
					return err
				}
			}
			if inText > 0 {
				if err := inline.WriteString(string(v)); err != nil {
					return err
				}
			}
		case xml.EndElement:
			switch v.Name.Local {
			case "v":
				inValue = false
			case "t":
				if inText > 0 {
					inText--
				}
			case "c":
				if !firstCell {
					if err := out.WriteString("\t"); err != nil {
						return err
					}
				}
				firstCell = false
				text := value.String()
				if cellType == "s" {
					i, parseErr := strconv.Atoi(strings.TrimSpace(text))
					if parseErr != nil || i < 0 || i >= len(shared) {
						return ErrMalformedDocument
					}
					text = shared[i]
				} else if cellType == "inlineStr" {
					text = inline.String()
				}
				if err := out.WriteString(text); err != nil {
					return err
				}
				inCell = false
			case "row":
				if inRow {
					if err := out.WriteString("\n"); err != nil {
						return err
					}
				}
				inRow = false
			}
		}
	}
}

func openXML(ctx context.Context, f *zip.File, limits Limits, budget *archiveBudget) (*xml.Decoder, func(), error) {
	r, err := f.Open()
	if err != nil {
		return nil, func() {}, fmt.Errorf("open office entry: %w", err)
	}
	entryLimit := limits.MaxEntryBytes
	if f.UncompressedSize64 < uint64(entryLimit) {
		entryLimit = int64(f.UncompressedSize64)
	}
	limited := &hardLimitReader{ctx: ctx, r: r, remaining: entryLimit, budget: budget}
	return xml.NewDecoder(limited), func() { _ = r.Close() }, nil
}

func documentError(operation string, err error) error {
	if errors.Is(err, ErrArchiveTooLarge) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("%s: %w", operation, ErrMalformedDocument)
}

func findFile(zr *zip.Reader, name string) *zip.File {
	for _, f := range zr.File {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func isWorksheet(name string) bool {
	return strings.HasPrefix(name, "xl/worksheets/") && !strings.Contains(name, "/_rels/") && path.Ext(name) == ".xml"
}

func attr(attrs []xml.Attr, local string) string {
	for _, a := range attrs {
		if a.Name.Local == local {
			return a.Value
		}
	}
	return ""
}
