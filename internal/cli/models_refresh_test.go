package cli

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRefreshV1ModelsWritesRawResponse(t *testing.T) {
	raw := []byte("{\n  \"data\": [{\"id\": \"model-a\"}]\n}\n")
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/v1/models" {
			t.Fatalf("path = %q, want /v1/models", r.URL.Path)
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
	})}

	destination := filepath.Join(t.TempDir(), "nested", "v1_models.json")
	if err := refreshV1Models(context.Background(), "http://llama/", destination, client); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(raw) {
		t.Fatalf("catalog = %q, want raw response %q", got, raw)
	}
}

func TestRefreshV1ModelsDoesNotReplaceOnHTTPError(t *testing.T) {
	destination := filepath.Join(t.TempDir(), "v1_models.json")
	original := []byte(`{"data":[{"id":"old"}]}`)
	if err := os.WriteFile(destination, original, 0o644); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
	})}

	if err := refreshV1Models(context.Background(), "http://llama", destination, client); err == nil {
		t.Fatal("expected HTTP error")
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("catalog changed after HTTP error: %q", got)
	}
}
