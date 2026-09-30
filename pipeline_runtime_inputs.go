package induction

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	kg "github.com/mwiater/induction/internal/knowledgegraph"
)

// preparePipelineRuntimeInputs creates the authoritative runtime collection
// exposed as inputs.documents.chunks. Existing document attachment handling is
// unchanged; this collection is additional structured pipeline input.
func preparePipelineRuntimeInputs(input *InputSet) (map[string]any, error) {
	result := map[string]any{"documents": map[string]any{"chunks": []any{}}}
	if input == nil {
		return result, nil
	}
	chunks := make([]any, 0)
	for _, path := range input.Documents {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read document %q: %w", path, err)
		}
		text, err := ExtractPDFText(path, DefaultAttachmentMaxBytes)
		if err != nil {
			return nil, fmt.Errorf("extract document %q: %w", path, err)
		}
		text = normalizePipelineDocumentText(text)
		docID := kg.DocumentID(data)
		for index, chunkText := range splitPipelineDocumentText(text, 6000) {
			chunkID := kg.ChunkID(docID, index, chunkText)
			chunks = append(chunks, map[string]any{"text": chunkText, "provenance": map[string]any{"document_id": docID, "chunk_id": chunkID, "chunk_index": index, "path": filepath.Clean(path), "display_name": filepath.Base(path), "sha256": sha256Hex(data)}})
		}
	}
	result["documents"].(map[string]any)["chunks"] = chunks
	return result, nil
}

func sha256Hex(data []byte) string { h := sha256.Sum256(data); return hex.EncodeToString(h[:]) }

// splitPipelineDocumentText returns exact contiguous substrings of the
// extracted text. It never normalizes or rewrites chunk contents, which keeps
// substring evidence verification authoritative.
func splitPipelineDocumentText(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}
	chunks := make([]string, 0, (len(text)+limit-1)/limit)
	for start := 0; start < len(text); {
		end := start + limit
		if end >= len(text) {
			chunks = append(chunks, text[start:])
			break
		}
		if boundary := strings.LastIndexAny(text[start:end], " \t\n\r"); boundary > limit/2 {
			end = start + boundary + 1
		}
		chunks = append(chunks, text[start:end])
		start = end
	}
	return chunks
}

// normalizePipelineDocumentText removes layout-only line wrapping emitted by
// PDF extraction while retaining paragraph boundaries. The resulting text is
// the authoritative runtime chunk text, so evidence verification remains an
// exact substring check against what the model actually received.
func normalizePipelineDocumentText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	var b strings.Builder
	blank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if b.Len() > 0 {
				blank = true
			}
			continue
		}
		if b.Len() > 0 {
			if blank {
				b.WriteString("\n\n")
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(line)
		blank = false
	}
	return b.String()
}
