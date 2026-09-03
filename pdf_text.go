package induction

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

// ExtractPDFText extracts text operators from a small, local PDF. It supports
// the common Flate-compressed streams used by generated reports and avoids
// adding a heavyweight PDF dependency to the client library.
func ExtractPDFText(path string, maxBytes int64) (string, error) {
	if maxBytes <= 0 {
		return "", errors.New("PDF size limit must be positive")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read PDF: %w", err)
	}
	if len(data) == 0 {
		return "", errors.New("PDF is empty")
	}
	if int64(len(data)) > maxBytes {
		return "", fmt.Errorf("PDF is too large: %d bytes exceeds %d-byte limit", len(data), maxBytes)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return "", errors.New("file is not a PDF")
	}

	var text strings.Builder
	for remaining := data; ; {
		start := bytes.Index(remaining, []byte("stream"))
		if start < 0 {
			break
		}
		remaining = remaining[start+len("stream"):]
		remaining = bytes.TrimLeft(remaining, " \t\r\n")
		end := bytes.Index(remaining, []byte("endstream"))
		if end < 0 {
			break
		}
		stream := remaining[:end]
		if decoded, ok := inflatePDFStream(stream); ok {
			for _, fragment := range pdfTextFragments(decoded) {
				if text.Len() > 0 {
					text.WriteByte(' ')
				}
				text.WriteString(fragment)
			}
		}
		remaining = remaining[end+len("endstream"):]
	}
	result := strings.TrimSpace(text.String())
	if result == "" {
		return "", errors.New("PDF contains no extractable text")
	}
	return result, nil
}

func inflatePDFStream(stream []byte) ([]byte, bool) {
	reader, err := zlib.NewReader(bytes.NewReader(stream))
	if err != nil {
		return nil, false
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	return decoded, err == nil
}

func pdfTextFragments(data []byte) []string {
	var fragments []string
	for i := 0; i < len(data); i++ {
		if data[i] != '(' {
			continue
		}
		value, next, ok := pdfLiteralString(data, i)
		if !ok {
			continue
		}
		trailing := data[next:]
		if bytes.Contains(trailing[:min(len(trailing), 24)], []byte("Tj")) || bytes.Contains(trailing[:min(len(trailing), 24)], []byte("TJ")) {
			if value = strings.TrimSpace(value); value != "" {
				fragments = append(fragments, value)
			}
		}
		i = next - 1
	}
	return fragments
}

func pdfLiteralString(data []byte, start int) (string, int, bool) {
	var value strings.Builder
	depth := 0
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '(':
			depth++
			if depth > 1 {
				value.WriteByte('(')
			}
		case ')':
			depth--
			if depth == 0 {
				return value.String(), i + 1, true
			}
			value.WriteByte(')')
		case '\\':
			if i+1 >= len(data) {
				return "", 0, false
			}
			i++
			switch data[i] {
			case 'n':
				value.WriteByte('\n')
			case 'r':
				value.WriteByte('\r')
			case 't':
				value.WriteByte('\t')
			case 'b':
				value.WriteByte('\b')
			case 'f':
				value.WriteByte('\f')
			default:
				value.WriteByte(data[i])
			}
		default:
			if depth > 0 {
				value.WriteByte(data[i])
			}
		}
	}
	return "", 0, false
}
