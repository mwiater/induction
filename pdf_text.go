package induction

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	pdf "github.com/giraffesyo/pdf"
)

// ExtractPDFText extracts selectable text from a PDF while preserving page and
// reading order. PDF text is not stored as a plain string: glyphs can be split
// across operators, encoded through a font-specific map, and positioned with
// coordinates. The PDF parser handles those details and reconstructs words
// from glyph positions.
func ExtractPDFText(path string, maxBytes int64) (string, error) {
	if maxBytes <= 0 {
		return "", errors.New("PDF size limit must be positive")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("read PDF: %w", err)
	}
	defer func() { _ = file.Close() }()

	info, err := file.Stat()
	if err != nil {
		return "", fmt.Errorf("stat PDF: %w", err)
	}
	if info.Size() == 0 {
		return "", errors.New("PDF is empty")
	}
	if info.Size() > maxBytes {
		return "", fmt.Errorf("PDF is too large: %d bytes exceeds %d-byte limit", info.Size(), maxBytes)
	}

	header := make([]byte, 5)
	if _, err := file.ReadAt(header, 0); err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read PDF header: %w", err)
	}
	if string(header) != "%PDF-" {
		return "", errors.New("file is not a PDF")
	}

	document, err := pdf.Extract(context.Background(), file, info.Size())
	if err != nil {
		return "", fmt.Errorf("parse PDF: %w", err)
	}
	result := strings.TrimSpace(document.Text())
	if result == "" {
		return "", errors.New("PDF contains no extractable text")
	}
	return result, nil
}
