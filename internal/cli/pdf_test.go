package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestPDFPreview(t *testing.T) {
	command := NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"pdf", "preview", "--file", "../../data/fixtures/documents/fixture.pdf"})

	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(output.String()) == "" {
		t.Fatal("expected extracted PDF text")
	}
}

func TestPDFPreviewRequiresFile(t *testing.T) {
	command := NewRootCommand()
	command.SetArgs([]string{"pdf", "preview"})

	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "required flag(s) \"file\"") {
		t.Fatalf("error=%v, want required file flag error", err)
	}
}
