// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

// markdownHeadingRE holds package-level state used internally by this package.
var markdownHeadingRE = regexp.MustCompile(`(?m)^# .+$`)

// validatePipelineQuality returns actionable feedback suitable for a retry
// prompt. It intentionally stays deterministic: a later chapter must never
// be accepted merely because a second model call said it was coherent.
func validatePipelineQuality(content string, quality *PipelineQuality) error {
	if quality == nil {
		return nil
	}
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return fmt.Errorf("output is empty")
	}
	words := countPipelineWords(trimmed)
	if quality.MinWords > 0 && words < quality.MinWords {
		return fmt.Errorf("output has %d words; minimum is %d", words, quality.MinWords)
	}
	if quality.MaxWords > 0 && words > quality.MaxWords {
		return fmt.Errorf("output has %d words; maximum is %d", words, quality.MaxWords)
	}
	if heading := strings.TrimSpace(quality.ExactHeading); heading != "" {
		lines := strings.Split(trimmed, "\n")
		first := ""
		for _, line := range lines {
			if strings.TrimSpace(line) != "" {
				first = strings.TrimSpace(line)
				break
			}
		}
		if first != heading {
			return fmt.Errorf("first heading must be %q", heading)
		}
		for _, found := range markdownHeadingRE.FindAllString(trimmed, -1) {
			if strings.TrimSpace(found) != heading {
				return fmt.Errorf("unexpected additional top-level heading %q", strings.TrimSpace(found))
			}
		}
	}
	lower := strings.ToLower(trimmed)
	for _, phrase := range quality.RequiredPhrases {
		if phrase != "" && !strings.Contains(lower, strings.ToLower(phrase)) {
			return fmt.Errorf("required continuity anchor %q is missing", phrase)
		}
	}
	for _, phrase := range quality.ForbiddenPhrases {
		if phrase != "" && strings.Contains(lower, strings.ToLower(phrase)) {
			return fmt.Errorf("forbidden planning phrase %q is present", phrase)
		}
	}
	return nil
}

// countPipelineWords performs the internal operation described by its signature.
func countPipelineWords(value string) int {
	return len(strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || r == '\u2014' }))
}

// pipelineQualityRetryFeedback performs the internal operation described by its signature.
func pipelineQualityRetryFeedback(quality *PipelineQuality, draft string, qualityErr error, attempt int) string {
	intro := fmt.Sprintf("QUALITY RETRY %d/%d: The previous draft failed validation: %s.", attempt, quality.MaxRetries, qualityErr)
	if quality.MinWords > 0 && countPipelineWords(draft) < quality.MinWords {
		length := fmt.Sprintf("at least %d words", quality.MinWords)
		if quality.MaxWords > 0 {
			length += fmt.Sprintf(" and at most %d words", quality.MaxWords)
		}
		return fmt.Sprintf("%s Expand the draft below into a complete replacement of %s. Develop the existing scenes with action, dialogue, and sensory detail; preserve the required heading and canon facts. Output only the complete revised text.\n\nPREVIOUS DRAFT:\n%s", intro, length, strings.TrimSpace(draft))
	}
	return intro + " Rewrite the complete output from the beginning. Preserve all required headings and canon facts; output only the finished text."
}
