// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	kg "github.com/mwiater/induction/internal/knowledgegraph"
)

// preparePipelineRuntimeContext adds live model metadata and step contracts to
// the template context. It is intentionally opt-in so ordinary pipelines do
// not depend on /v1/models being available.
func preparePipelineRuntimeContext(ctx context.Context, client *Client, pipeline *Pipeline, inputs map[string]any) error {
	steps := make(map[string]any)
	needsCatalog := false
	for _, step := range pipeline.Steps {
		steps[step.Name] = map[string]any{"contract": step.Contract, "quality": step.Quality}
		values := []string{step.Model, step.UserPrompt, step.SystemPrompt, step.ForEach}
		if step.Decision != nil {
			values = append(values, step.Decision.StateString(), step.Decision.Instructions, step.Decision.CandidatesFrom)
		}
		for _, value := range values {
			if strings.Contains(value, "runtime.active_model_catalog") || strings.Contains(value, "runtime.text_generation_models") {
				needsCatalog = true
			}
		}
	}
	inputs["steps"] = steps
	if !needsCatalog {
		return nil
	}
	if client == nil {
		return fmt.Errorf("pipeline runtime model catalog requires a client")
	}
	catalog, err := client.ListModelCatalog(ctx)
	if err != nil {
		return fmt.Errorf("load runtime model catalog: %w", err)
	}
	all := make(map[string]any, len(catalog))
	text := make(map[string]any, len(catalog))
	for _, model := range catalog {
		metadata := map[string]any{"id": model.ID, "capabilities": model.Capabilities, "inputModalities": model.InputModes, "outputModalities": model.OutputModes}
		all[model.ID] = metadata
		if !hasOutputMode(model.OutputModes, "decisions") && !hasOutputMode(model.OutputModes, "embeddings") {
			text[model.ID] = modelCatalogSummary(model)
		}
	}
	inputs["runtime"] = map[string]any{"active_model_catalog": all, "text_generation_models": text}
	return nil
}

// StateString is an exported operation provided by this package.
func (d DecisionConfig) StateString() string {
	if value, ok := d.State.(string); ok {
		return value
	}
	data, _ := json.Marshal(d.State)
	return string(data)
}

// hasOutputMode performs the internal operation described by its signature.
func hasOutputMode(modes []string, wanted string) bool {
	for _, mode := range modes {
		if strings.EqualFold(mode, wanted) {
			return true
		}
	}
	return false
}

// modelCatalogSummary performs the internal operation described by its signature.
func modelCatalogSummary(model ModelCatalogEntry) string {
	features := []string{"text generation"}
	if model.Capabilities.ImageInput || hasOutputMode(model.InputModes, "image") || hasOutputMode(model.InputModes, "vision") {
		features = append(features, "vision input")
	}
	if model.Capabilities.AudioInput || hasOutputMode(model.InputModes, "audio") {
		features = append(features, "audio input")
	}
	return strings.Join(features, ", ")
}

// preparePipelineRuntimeInputs creates the authoritative runtime collection
// exposed as inputs.documents.chunks. Existing document attachment handling is
// unchanged; this collection is additional structured pipeline input.
func preparePipelineRuntimeInputs(input *InputSet) (map[string]any, error) {
	result := map[string]any{"text": "", "documents": map[string]any{"chunks": []any{}}}
	if input == nil {
		return result, nil
	}
	result["text"] = input.Text
	chunks := make([]any, 0)
	for _, path := range input.Documents {
		data, err := attachmentBytes(path, DefaultAttachmentMaxBytes)
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

// sha256Hex performs the internal operation described by its signature.
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
