package induction

import (
	"encoding/json"
	"testing"
)

func TestTextConcatTransformResolvesOrderedParts(t *testing.T) {
	step := PipelineStep{
		Transform: "text.concat",
		Input: map[string]any{
			"parts": []any{
				"{{ steps.one.output }}",
				"{{ steps.two.output }}",
			},
		},
	}
	got, err := executePipelineTransform(step, map[string]json.RawMessage{
		"one": json.RawMessage(`"# Chapter 1"`),
		"two": json.RawMessage(`"# Chapter 2"`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# Chapter 1\n\n# Chapter 2" {
		t.Fatalf("assembled text = %q", got)
	}
}

func TestTextSelectTransform(t *testing.T) {
	step := PipelineStep{
		Transform: "text.select",
		Input: map[string]any{
			"decision": "{{ steps.route.output }}",
			"choices": map[string]any{
				"pass":   "approved packet",
				"repair": "repaired packet",
			},
		},
	}
	got, err := executePipelineTransform(step, map[string]json.RawMessage{
		"route": json.RawMessage(`{"type":"decision","selectedValue":"repair"}`),
	})
	if err != nil {
		t.Fatalf("text.select failed: %v", err)
	}
	if string(got) != "repaired packet" {
		t.Fatalf("text.select = %q", got)
	}
}
