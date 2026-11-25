package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestListApplicationTools(t *testing.T) {
	root := NewRootCommand()
	var output bytes.Buffer
	root.SetOut(&output)
	root.SetErr(&output)
	root.SetArgs([]string{"list", "applicationTools"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	for _, tool := range applicationToolDefinitions() {
		line := tool.Name + ": " + tool.Description
		if !strings.Contains(text, line) {
			t.Fatalf("missing application tool line %q in output:\n%s", line, text)
		}
	}
}
