// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// PipelineRetry opts one generator step into bounded deterministic validation
// and optional next-token JEV review. It is deliberately separate from the
// legacy decision/when routing feature.
// PipelineRetry is an exported type in this package's public data model.
type PipelineRetry struct {
	// MaxAttempts stores the value represented by this member.
	MaxAttempts int `yaml:"maxAttempts"`
	// OnExhausted stores the value represented by this member.
	OnExhausted string `yaml:"onExhausted,omitempty"`
	// MinConfidence stores the value represented by this member.
	MinConfidence float64 `yaml:"minConfidence,omitempty"`
	// OnUncertain stores the value represented by this member.
	OnUncertain string `yaml:"onUncertain,omitempty"`
	// OnDecisionError stores the value represented by this member.
	OnDecisionError string `yaml:"onDecisionError,omitempty"`
	// MaxRepairFeedbackChars stores the value represented by this member.
	MaxRepairFeedbackChars int `yaml:"maxRepairFeedbackChars,omitempty"`
	// Validate stores the value represented by this member.
	Validate *PipelineValidation `yaml:"validate,omitempty"`
	// QualityGate stores the value represented by this member.
	QualityGate *PipelineQualityGate `yaml:"qualityGate,omitempty"`
	// SchemaValue stores the value represented by this member.
	SchemaValue map[string]any `yaml:"-"`
}

// PipelineValidation is an exported type in this package's public data model.
type PipelineValidation struct {
	// JSON stores the value represented by this member.
	JSON bool `yaml:"json,omitempty"`
	// Schema stores the value represented by this member.
	Schema string `yaml:"schema,omitempty"`
	// SchemaFile stores the value represented by this member.
	SchemaFile string `yaml:"schemaFile,omitempty"`
	// Rules stores the value represented by this member.
	Rules []PipelineValidationRule `yaml:"rules,omitempty"`
}

// PipelineValidationRule is an exported type in this package's public data model.
type PipelineValidationRule struct {
	// Type stores the value represented by this member.
	Type string `yaml:"type"`
	// Path stores the value represented by this member.
	Path string `yaml:"path,omitempty"`
	// Value stores the value represented by this member.
	Value int `yaml:"value,omitempty"`
	// Field stores the value represented by this member.
	Field string `yaml:"field,omitempty"`
}

// PipelineQualityGate is an exported type in this package's public data model.
type PipelineQualityGate struct {
	// Model stores the value represented by this member.
	Model string `yaml:"model"`
	// UseMCP stores the value represented by this member.
	UseMCP bool `yaml:"usemcp,omitempty"`
	// TopLogprobs stores the value represented by this member.
	TopLogprobs int `yaml:"topLogprobs,omitempty"`
	// Candidates stores the value represented by this member.
	Candidates map[string]string `yaml:"candidates"`
	// SystemPrompt stores the value represented by this member.
	SystemPrompt string `yaml:"systemPrompt,omitempty"`
	// UserPrompt stores the value represented by this member.
	UserPrompt string `yaml:"userPrompt"`
}

// normalizePipelineRetry performs the internal operation described by its signature.
func normalizePipelineRetry(r *PipelineRetry) {
	if r.OnExhausted == "" {
		r.OnExhausted = "fail"
	}
	if r.MinConfidence == 0 {
		r.MinConfidence = 0.55
	}
	if r.OnUncertain == "" {
		r.OnUncertain = "repair"
	}
	if r.OnDecisionError == "" {
		r.OnDecisionError = "regenerate"
	}
	if r.MaxRepairFeedbackChars == 0 {
		r.MaxRepairFeedbackChars = 3000
	}
	if r.QualityGate != nil && r.QualityGate.TopLogprobs == 0 {
		r.QualityGate.TopLogprobs = 20
	}
}

// validatePipelineRetry performs the internal operation described by its signature.
func validatePipelineRetry(step PipelineStep, base string) error {
	r := step.Retry
	if r == nil {
		return nil
	}
	if step.Transform != "" || effectiveDecision(step) != nil {
		return fmt.Errorf("retry is only allowed on generator steps")
	}
	if step.UseMCP || step.Image != "" || step.Document != "" {
		return fmt.Errorf("retry requires MCP to be disabled and cannot replay attachments")
	}
	if r.MaxAttempts < 1 {
		return fmt.Errorf("retry.maxAttempts must be at least 1")
	}
	onExhausted, onUncertain, onDecisionError := r.OnExhausted, r.OnUncertain, r.OnDecisionError
	if onExhausted == "" {
		onExhausted = "fail"
	}
	if onUncertain == "" {
		onUncertain = "repair"
	}
	if onDecisionError == "" {
		onDecisionError = "regenerate"
	}
	for field, value := range map[string]string{"onExhausted": onExhausted, "onUncertain": onUncertain, "onDecisionError": onDecisionError} {
		if field == "onExhausted" && value != "fail" && value != "acceptValidWithWarning" {
			return fmt.Errorf("retry.%s must be fail or acceptValidWithWarning", field)
		}
		if field != "onExhausted" && value != "repair" && value != "regenerate" && value != "fail" {
			return fmt.Errorf("retry.%s has unsupported value %q", field, value)
		}
	}
	if r.MinConfidence < 0 || r.MinConfidence > 1 {
		return fmt.Errorf("retry.minConfidence must be between 0 and 1")
	}
	maxRepairFeedbackChars := r.MaxRepairFeedbackChars
	if maxRepairFeedbackChars == 0 {
		maxRepairFeedbackChars = 3000
	}
	if maxRepairFeedbackChars < 1 {
		return fmt.Errorf("retry.maxRepairFeedbackChars must be positive")
	}
	if r.Validate == nil && r.QualityGate == nil {
		return fmt.Errorf("retry requires validate or qualityGate")
	}
	if r.Validate != nil {
		if r.Validate.Schema != "" && r.Validate.SchemaFile != "" {
			return fmt.Errorf("validate.schema and validate.schemaFile are mutually exclusive")
		}
		if r.Validate.SchemaFile != "" {
			path := r.Validate.SchemaFile
			if !filepath.IsAbs(path) {
				path = filepath.Join(base, path)
			}
			data, err := os.ReadFile(filepath.Clean(path))
			if err != nil {
				return fmt.Errorf("read retry schema %q: %w", path, err)
			}
			r.Validate.Schema = string(data)
		}
		if r.Validate.Schema != "" {
			var schema map[string]any
			if err := json.Unmarshal([]byte(r.Validate.Schema), &schema); err != nil || schema == nil {
				return fmt.Errorf("retry schema is invalid JSON")
			}
			r.SchemaValue = schema
			r.Validate.JSON = true
		}
		for _, rule := range r.Validate.Rules {
			switch rule.Type {
			case "minWords", "maxWords":
				if rule.Value < 0 {
					return fmt.Errorf("retry rule %s value must be non-negative", rule.Type)
				}
			case "uniqueJsonPointers":
				if rule.Path == "" || strings.TrimSpace(rule.Field) == "" {
					return fmt.Errorf("uniqueJsonPointers requires path and field")
				}
			default:
				return fmt.Errorf("unsupported retry validation rule %q", rule.Type)
			}
		}
	}
	if r.QualityGate != nil {
		g := r.QualityGate
		if strings.TrimSpace(g.Model) == "" || strings.TrimSpace(g.UserPrompt) == "" {
			return fmt.Errorf("qualityGate.model and qualityGate.userPrompt are required")
		}
		if g.UseMCP {
			return fmt.Errorf("qualityGate cannot use MCP")
		}
		if len(g.Candidates) != 3 {
			return fmt.Errorf("qualityGate requires pass, repair, and regenerate candidates")
		}
		seen := map[string]bool{}
		for key, label := range g.Candidates {
			if seen[label] || (label != "pass" && label != "repair" && label != "regenerate") {
				return fmt.Errorf("qualityGate candidates must have unique pass, repair, and regenerate labels")
			}
			seen[label] = true
			if strings.TrimSpace(key) == "" {
				return fmt.Errorf("qualityGate candidate key cannot be empty")
			}
		}
		if !seen["pass"] || !seen["repair"] || !seen["regenerate"] {
			return fmt.Errorf("qualityGate must define pass, repair, and regenerate")
		}
		topLogprobs := g.TopLogprobs
		if topLogprobs == 0 {
			topLogprobs = 20
		}
		if topLogprobs < len(g.Candidates) {
			return fmt.Errorf("qualityGate.topLogprobs must be at least the candidate count")
		}
	}
	return nil
}

// PipelineValidationIssue is an exported type in this package's public data model.
type PipelineValidationIssue struct { // Message explains the validation failure.
	// Path stores the value represented by this member.
	// Code stores the value represented by this member.
	Code, Path, Message string
}

// String is an exported operation provided by this package.
func (i PipelineValidationIssue) String() string {
	if i.Path == "" {
		return i.Code + ": " + i.Message
	}
	return i.Code + " at " + i.Path + ": " + i.Message
}

// validateRetryOutput performs the internal operation described by its signature.
func validateRetryOutput(content string, retry *PipelineRetry) []PipelineValidationIssue {
	if strings.TrimSpace(content) == "" {
		return []PipelineValidationIssue{{Code: "empty_output", Message: "output is empty"}}
	}
	if retry == nil || retry.Validate == nil {
		return nil
	}
	v := retry.Validate
	var root any
	needsJSON := v.JSON || retry.SchemaValue != nil
	for _, rule := range v.Rules {
		if rule.Type == "uniqueJsonPointers" || rule.Path != "" {
			needsJSON = true
		}
	}
	if needsJSON {
		decoder := json.NewDecoder(strings.NewReader(content))
		decoder.UseNumber()
		if err := decoder.Decode(&root); err != nil {
			return []PipelineValidationIssue{{Code: "json_syntax", Message: err.Error()}}
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return []PipelineValidationIssue{{Code: "json_syntax", Message: "trailing JSON content"}}
		}
	} else {
		root = content
	}
	issues := make([]PipelineValidationIssue, 0, 8)
	if retry.SchemaValue != nil {
		validateSchemaValue(retry.SchemaValue, root, "", &issues)
	}
	for _, rule := range v.Rules {
		value, ok := pipelineJSONPointer(root, rule.Path)
		switch rule.Type {
		case "minWords", "maxWords":
			if !ok {
				issues = append(issues, PipelineValidationIssue{"rule_violation", rule.Path, "JSON pointer not found"})
				continue
			}
			text, ok := value.(string)
			if !ok {
				issues = append(issues, PipelineValidationIssue{"rule_violation", rule.Path, "value must be a string"})
				continue
			}
			words := len(strings.FieldsFunc(text, unicode.IsSpace))
			if rule.Type == "minWords" && words < rule.Value {
				issues = append(issues, PipelineValidationIssue{"min_words", rule.Path, fmt.Sprintf("has %d words; minimum is %d", words, rule.Value)})
			}
			if rule.Type == "maxWords" && words > rule.Value {
				issues = append(issues, PipelineValidationIssue{"max_words", rule.Path, fmt.Sprintf("has %d words; maximum is %d", words, rule.Value)})
			}
		case "uniqueJsonPointers":
			if !ok {
				issues = append(issues, PipelineValidationIssue{"rule_violation", rule.Path, "JSON pointer not found"})
				continue
			}
			items, ok := value.([]any)
			if !ok {
				issues = append(issues, PipelineValidationIssue{"rule_violation", rule.Path, "value must be an array"})
				continue
			}
			seen := map[string]bool{}
			for n, item := range items {
				obj, ok := item.(map[string]any)
				if !ok {
					issues = append(issues, PipelineValidationIssue{"rule_violation", fmt.Sprintf("%s/%d", rule.Path, n), "item must be an object"})
					continue
				}
				text, ok := obj[rule.Field].(string)
				if !ok || text == "" {
					issues = append(issues, PipelineValidationIssue{"rule_violation", fmt.Sprintf("%s/%d/%s", rule.Path, n, rule.Field), "field must be a non-empty string"})
					continue
				}
				if seen[text] {
					issues = append(issues, PipelineValidationIssue{"duplicate_id", fmt.Sprintf("%s/%d/%s", rule.Path, n, rule.Field), "value is duplicated"})
				}
				seen[text] = true
			}
		}
	}
	return issues
}

// pipelineJSONPointer performs the internal operation described by its signature.
func pipelineJSONPointer(root any, pointer string) (any, bool) {
	if pointer == "" {
		return root, true
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, false
	}
	current := root
	for _, raw := range strings.Split(pointer[1:], "/") {
		token := strings.ReplaceAll(strings.ReplaceAll(raw, "~1", "/"), "~0", "~")
		switch value := current.(type) {
		case map[string]any:
			var ok bool
			current, ok = value[token]
			if !ok {
				return nil, false
			}
		case []any:
			var index int
			if _, err := fmt.Sscanf(token, "%d", &index); err != nil || index < 0 || index >= len(value) {
				return nil, false
			}
			current = value[index]
		default:
			return nil, false
		}
	}
	return current, true
}

// validateSchemaValue performs the internal operation described by its signature.
func validateSchemaValue(schema map[string]any, value any, path string, issues *[]PipelineValidationIssue) {
	typeName, _ := schema["type"].(string)
	validType := true
	switch typeName {
	case "object":
		_, validType = value.(map[string]any)
	case "array":
		_, validType = value.([]any)
	case "string":
		_, validType = value.(string)
	}
	if !validType {
		*issues = append(*issues, PipelineValidationIssue{"schema", path, "type must be " + typeName})
		return
	}
	if obj, ok := value.(map[string]any); ok {
		if required, ok := schema["required"].([]any); ok {
			for _, item := range required {
				name, _ := item.(string)
				if _, exists := obj[name]; !exists {
					*issues = append(*issues, PipelineValidationIssue{"schema", path + "/" + name, "required property is missing"})
				}
			}
		}
		properties, _ := schema["properties"].(map[string]any)
		additional, _ := schema["additionalProperties"].(bool)
		for name, child := range obj {
			raw, exists := properties[name]
			if !exists {
				if additional {
					continue
				}
				*issues = append(*issues, PipelineValidationIssue{"schema", path + "/" + name, "additional property is not allowed"})
				continue
			}
			childSchema, ok := raw.(map[string]any)
			if ok {
				validateSchemaValue(childSchema, child, path+"/"+name, issues)
			}
		}
	}
	if array, ok := value.([]any); ok {
		if minItems, ok := schema["minItems"]; ok && len(array) < numberIntValue(minItems) {
			*issues = append(*issues, PipelineValidationIssue{"schema", path, "array has too few items"})
		}
		if child, ok := schema["items"].(map[string]any); ok {
			for n, item := range array {
				validateSchemaValue(child, item, fmt.Sprintf("%s/%d", path, n), issues)
			}
		}
	}
	if text, ok := value.(string); ok {
		if minLength, ok := schema["minLength"]; ok && len([]rune(text)) < numberIntValue(minLength) {
			*issues = append(*issues, PipelineValidationIssue{"schema", path, "string is too short"})
		}
	}
}

// numberIntValue performs the internal operation described by its signature.
func numberIntValue(raw any) int {
	switch n := raw.(type) {
	case json.Number:
		var value int64
		_, _ = fmt.Sscan(n.String(), &value)
		return int(value)
	case float64:
		return int(n)
	case int:
		return n
	}
	return 0
}

// formatRetryIssues performs the internal operation described by its signature.
func formatRetryIssues(issues []PipelineValidationIssue, limit int) string {
	if len(issues) == 0 {
		return ""
	}
	var b bytes.Buffer
	for n, issue := range issues {
		if n >= 20 {
			b.WriteString("\n- additional validation issues omitted")
			break
		}
		text := issue.String()
		if len(text) > 500 {
			text = text[:500] + "..."
		}
		if b.Len()+len(text)+3 > limit {
			b.WriteString("\n- additional diagnostics omitted")
			break
		}
		b.WriteString("\n- ")
		b.WriteString(text)
	}
	return b.String()
}
