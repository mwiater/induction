package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/mwiater/induction"
)

func TestRequestReferencesUploadedDocument(t *testing.T) {
	req := request("test-model", "The extracted PDF text.", "fixture.pdf")
	parts := req.Messages[0].Content.([]induction.ContentPart)
	if len(parts) != 2 || !strings.Contains(parts[1].Text, "The extracted PDF text.") || !strings.Contains(parts[1].Text, "fixture.pdf") {
		t.Fatalf("parts=%#v", parts)
	}
	body, err := json.Marshal(req)
	if err != nil || !strings.Contains(string(body), "Extracted document text") {
		t.Fatalf("body=%s err=%v", body, err)
	}
}
