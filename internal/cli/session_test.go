package cli

import (
	"bytes"
	"strings"
	"testing"

	induction "github.com/mwiater/induction"
)

func TestRenderSessionTranscript(t *testing.T) {
	session := &induction.ChatSession{
		Messages: []induction.Message{{Role: "system", Content: "hidden"}},
		Snapshots: []*induction.ModelSnapshot{{
			Messages: []induction.Message{
				{Role: "system", Content: "hidden"},
				{Role: "user", Content: "hello"},
				{Role: "assistant", Content: "answer"},
			},
			Interaction: []induction.Interaction{{Content: "answer", ReasoningContent: "thinking"}},
		}},
	}
	var out bytes.Buffer
	if err := induction.RenderSessionTranscript(&out, session); err != nil {
		t.Fatal(err)
	}
	result := out.String()
	if !strings.Contains(result, "★ You: hello") || !strings.Contains(result, "✦ Assistant: <think>thinking</think>answer") {
		t.Fatalf("unexpected transcript: %q", result)
	}
	if strings.Contains(result, "hidden") {
		t.Fatalf("system message leaked into transcript: %q", result)
	}
}
