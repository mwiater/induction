package induction

import (
	"fmt"
	"path/filepath"
	"strings"
)

// inputContent prepares the first UI pipeline turn from a complete input set.
// It only prepares attachment content; inference remains owned by Bubble Tea.
func inputContent(input InputSet, first PipelineStep) (any, string, string, error) {
	paths := input.allPaths()
	if len(paths) == 0 {
		if first.Image != "" {
			paths = []string{first.Image}
			input.Images = []string{first.Image}
		}
		if first.Document != "" {
			paths = []string{first.Document}
			input.Documents = []string{first.Document}
		}
	}
	parts := []ContentPart{{Type: "text", Text: first.UserPrompt}}
	var images, docs []string
	for _, path := range paths {
		isDoc := false
		for _, candidate := range input.Documents {
			if candidate == path {
				isDoc = true
				break
			}
		}
		if !isDoc && len(input.Images) == 0 && filepath.Ext(path) == ".pdf" {
			isDoc = true
		}
		if isDoc {
			text, err := ExtractPDFText(path, DefaultAttachmentMaxBytes)
			if err != nil {
				return nil, "", "", fmt.Errorf("prepare document %q: %w", path, err)
			}
			_, filename, err := FileDataURL(path, DefaultAttachmentMaxBytes)
			if err != nil {
				return nil, "", "", err
			}
			docs = append(docs, filepath.Base(path))
			parts = append(parts, ContentPart{Type: "text", Text: "Document filename: " + filename + "\n\nExtracted document text:\n" + text})
		} else {
			data, err := ImageDataURL(path, DefaultAttachmentMaxBytes)
			if err != nil {
				return nil, "", "", fmt.Errorf("prepare image %q: %w", path, err)
			}
			images = append(images, filepath.Base(path))
			parts = append(parts, ContentPart{Type: "image_url", ImageURL: &ImageURLPart{URL: data, Detail: "low"}})
		}
	}
	return parts, strings.Join(images, ", "), strings.Join(docs, ", "), nil
}
