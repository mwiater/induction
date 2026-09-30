package induction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// renderPipelineReferences supports the intentionally small artifact syntax.
// Values are inserted as compact JSON; plain strings are inserted unchanged.
func renderPipelineReferences(prompt string, outputs map[string]json.RawMessage, item any) (string, error) {
	return renderPipelineReferencesFor(prompt, outputs, nil, item, "item")
}

func renderPipelineReferencesFor(prompt string, outputs map[string]json.RawMessage, inputs map[string]any, item any, itemName string) (string, error) {
	for {
		start := strings.Index(prompt, "{{")
		if start < 0 {
			return prompt, nil
		}
		end := strings.Index(prompt[start+2:], "}}")
		if end < 0 {
			return "", fmt.Errorf("unterminated reference")
		}
		end += start + 2
		expr := strings.TrimSpace(prompt[start+2 : end])
		value, err := resolvePipelineReferenceFor(expr, outputs, inputs, item, itemName)
		if err != nil {
			return "", err
		}
		var replacement string
		switch v := value.(type) {
		case string:
			replacement = v
		default:
			data, err := json.Marshal(v)
			if err != nil {
				return "", err
			}
			replacement = string(data)
		}
		prompt = prompt[:start] + replacement + prompt[end+2:]
	}
}

func resolvePipelineReference(expr string, outputs map[string]json.RawMessage, item any) (any, error) {
	return resolvePipelineReferenceFor(expr, outputs, nil, item, "item")
}

func resolvePipelineReferenceFor(expr string, outputs map[string]json.RawMessage, inputs map[string]any, item any, itemName string) (any, error) {
	if expr == "item" || expr == itemName {
		if item == nil {
			return nil, fmt.Errorf("item is not available outside fan-out")
		}
		return item, nil
	}
	if item != nil && itemName != "" && strings.HasPrefix(expr, itemName+".") {
		value := item
		for _, field := range strings.Split(strings.TrimPrefix(expr, itemName+"."), ".") {
			obj, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("reference %q traverses a non-object", expr)
			}
			value, ok = obj[field]
			if !ok {
				return nil, fmt.Errorf("reference %q field %q is missing", expr, field)
			}
		}
		return value, nil
	}
	if strings.HasPrefix(expr, "inputs.") {
		parts := strings.Split(expr, ".")
		// Runtime inputs are normally passed as the direct map containing
		// "documents". Accept the wrapped form as well so callers that expose
		// the complete template context remain compatible.
		if root, ok := inputs["inputs"].(map[string]any); ok {
			inputs = root
		}
		var value any = inputs
		for _, field := range parts[1:] {
			obj, ok := value.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("reference %q traverses a non-object", expr)
			}
			value, ok = obj[field]
			if !ok {
				return nil, fmt.Errorf("reference %q field %q is missing", expr, field)
			}
		}
		return value, nil
	}
	if !strings.HasPrefix(expr, "steps.") {
		return nil, fmt.Errorf("unsupported reference %q", expr)
	}
	parts := strings.Split(expr, ".")
	if len(parts) < 3 {
		return nil, fmt.Errorf("unsupported reference %q", expr)
	}
	raw, ok := outputs[parts[1]]
	if !ok {
		return nil, fmt.Errorf("step %q has no completed output", parts[1])
	}
	if parts[2] == "items" {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return nil, err
		}
		return value, nil
	}
	if parts[2] != "output" {
		return nil, fmt.Errorf("unsupported reference %q", expr)
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	for _, field := range parts[3:] {
		obj, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("reference %q traverses a non-object", expr)
		}
		value, ok = obj[field]
		if !ok {
			return nil, fmt.Errorf("reference %q field %q is missing", expr, field)
		}
	}
	return value, nil
}

func compactJSON(data []byte) string {
	var b bytes.Buffer
	if json.Compact(&b, data) == nil {
		return b.String()
	}
	return string(data)
}
