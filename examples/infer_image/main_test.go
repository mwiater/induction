package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/mwiater/induction"
)

func TestRequestPreservesContentOrder(t *testing.T) {
	req, err := request("test-model", "../../examples/assets/images/fixture.jpg")
	if err != nil {
		t.Fatal(err)
	}
	parts, ok := req.Messages[0].Content.([]induction.ContentPart)
	if !ok || len(parts) != 2 || parts[0].Type != "text" || !strings.Contains(parts[0].Text, "Subject Matter:") || !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/jpeg") {
		t.Fatalf("parts=%#v", req.Messages[0].Content)
	}
	body, _ := json.Marshal(req.Messages[0].Content)
	if !bytes.Contains(body, []byte(`"detail":"low"`)) {
		t.Fatalf("body=%s", body)
	}
}
