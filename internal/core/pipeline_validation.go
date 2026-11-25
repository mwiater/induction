// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// resolveInputSetPaths performs the internal operation described by its signature.
func resolveInputSetPaths(input *InputSet, base string) {
	if input == nil {
		return
	}
	set := input
	for _, values := range [][]string{set.Files, set.Images, set.Documents} {
		for i, value := range values {
			if value != "" && !filepath.IsAbs(value) && !IsRemoteSource(value) {
				values[i] = filepath.Join(base, value)
			}
		}
	}
}

// validatePipelineReferences deliberately accepts only the small reference
// language used by pipeline artifacts. It catches future-step references
// before Bubble Tea starts model loading.
func validatePipelineReferences(step PipelineStep, index int, prior map[string]bool, positions map[string]int) error {
	values := []string{step.Model, step.UserPrompt, step.SystemPrompt, step.ForEach}
	values = append(values, step.Contract...)
	if step.Decision != nil {
		values = append(values, step.Decision.Instructions)
		values = append(values, step.Decision.CandidatesFrom)
		if state, ok := step.Decision.State.(string); ok {
			values = append(values, state)
		}
	}
	for _, value := range values {
		for pos := 0; pos < len(value); {
			start := strings.Index(value[pos:], "{{")
			if start < 0 {
				break
			}
			start += pos
			end := strings.Index(value[start+2:], "}}")
			if end < 0 {
				return fmt.Errorf("steps[%d]: unterminated reference", index)
			}
			end += start + 2
			expr := strings.TrimSpace(value[start+2 : end])
			if expr == "item" || expr == step.As || strings.HasPrefix(expr, step.As+".") || strings.HasPrefix(expr, "inputs.text") || strings.HasPrefix(expr, "inputs.documents.chunks") || strings.HasPrefix(expr, "inputs.prompt") || strings.HasPrefix(expr, "runtime.") {
				pos = end + 2
				continue
			}
			if !strings.HasPrefix(expr, "steps.") {
				return fmt.Errorf("steps[%d]: unsupported reference %q", index, expr)
			}
			parts := strings.Split(expr, ".")
			if len(parts) < 3 || parts[2] != "output" && parts[2] != "items" && parts[2] != "contract" && parts[2] != "quality" {
				return fmt.Errorf("steps[%d]: unsupported reference %q", index, expr)
			}
			name := parts[1]
			if !prior[name] {
				if position, exists := positions[name]; exists && position >= index {
					return fmt.Errorf("steps[%d]: reference to future step %q", index, name)
				}
				return fmt.Errorf("steps[%d]: unknown step reference %q", index, name)
			}
			pos = end + 2
		}
	}
	for key, value := range step.Input {
		if text, ok := value.(string); ok {
			if err := validatePipelineReferences(PipelineStep{UserPrompt: text}, index, prior, positions); err != nil {
				return fmt.Errorf("steps[%d].input[%q]: %w", index, key, err)
			}
		}
	}
	return nil
}

// Validate is an exported operation provided by this package.
// Additional context: HTTP(S) sources are validated when they are fetched by the attachment processor. They cannot be checked with os.Stat.
func (s InputSet) Validate(label string) error {
	if len(s.Files)+len(s.Images)+len(s.Documents) == 0 && strings.TrimSpace(s.Text) == "" {
		return fmt.Errorf("%s must contain at least one input", label)
	}
	for _, path := range append(append(append([]string{}, s.Files...), s.Images...), s.Documents...) {
		if strings.TrimSpace(path) == "" {
			return fmt.Errorf("%s contains an empty input path", label)
		}

		if IsRemoteSource(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("%s input %q: %w", label, path, err)
		}
		if info.IsDir() {
			return fmt.Errorf("%s input %q is a directory", label, path)
		}
		if info.Size() == 0 {
			return fmt.Errorf("%s input %q is empty", label, path)
		}
	}
	return nil
}

// DerivedID is an exported operation provided by this package.
func (s InputSet) DerivedID(index int) string {
	paths := append(append(append([]string{}, s.Files...), s.Images...), s.Documents...)
	if len(paths) > 0 {
		return filepath.ToSlash(filepath.Clean(paths[0]))
	}
	return fmt.Sprintf("item-%03d", index+1)
}
