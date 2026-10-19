package induction

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

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

func countPipelineWords(value string) int {
	return len(strings.FieldsFunc(value, func(r rune) bool { return unicode.IsSpace(r) || r == '\u2014' }))
}
