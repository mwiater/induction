package induction

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestContentPartsMarshalInOrder(t *testing.T) {
	req := ChatRequest{Messages: []Message{{Role: "user", Content: []ContentPart{
		{Type: "text", Text: "before"},
		{Type: "image_url", ImageURL: &ImageURLPart{URL: "https://example.test/a.png", Detail: "high"}},
		{Type: "file", File: &FileContentPart{FileID: "file-1", Filename: "notes.txt"}},
	}}}}
	body, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"messages":[{"role":"user","content":[{"type":"text","text":"before"},{"type":"image_url","image_url":{"url":"https://example.test/a.png","detail":"high"}},{"type":"file","file":{"file_id":"file-1","filename":"notes.txt"}}]}]}`
	if string(body) != want {
		t.Fatalf("JSON=%s, want %s", body, want)
	}
}

func TestAttachmentDataURLsValidate(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "image.svg")
	if err := os.WriteFile(image, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), 0600); err != nil {
		t.Fatal(err)
	}
	url, err := ImageDataURL(image, 1000)
	if err != nil || !strings.HasPrefix(url, "data:image/svg+xml;base64,") {
		t.Fatalf("url=%q err=%v", url, err)
	}
	empty := filepath.Join(dir, "empty.txt")
	if err := os.WriteFile(empty, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := FileDataURL(empty, 1000); err == nil {
		t.Fatal("expected empty-file error")
	}
	bad := filepath.Join(dir, "bad.bin")
	if err := os.WriteFile(bad, []byte("not a document"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := FileDataURL(bad, 1000); err == nil {
		t.Fatal("expected MIME error")
	}
}

func TestRepositoryJPEGAndPDFFixtures(t *testing.T) {
	imageURL, err := ImageDataURL("examples/assets/images/fixture.jpg", DefaultAttachmentMaxBytes)
	if err != nil || !strings.HasPrefix(imageURL, "data:image/jpeg;base64,") {
		t.Fatalf("JPEG data URL=%q err=%v", imageURL[:min(len(imageURL), 32)], err)
	}
	fileURL, filename, err := FileDataURL("examples/assets/documents/fixture.pdf", DefaultAttachmentMaxBytes)
	if err != nil || filename != "fixture.pdf" || !strings.HasPrefix(fileURL, "data:application/pdf;base64,") {
		t.Fatalf("PDF data URL=%q filename=%q err=%v", fileURL[:min(len(fileURL), 32)], filename, err)
	}
	text, err := ExtractPDFText("examples/assets/documents/fixture.pdf", DefaultAttachmentMaxBytes)
	if err != nil || strings.TrimSpace(text) == "" {
		t.Fatalf("PDF text=%q err=%v", text, err)
	}
}

func TestUploadAndDeleteFile(t *testing.T) {
	var uploaded bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			if r.URL.Path != "/v1/files" {
				t.Fatalf("path=%s", r.URL.Path)
			}
			reader, err := r.MultipartReader()
			if err != nil {
				t.Fatal(err)
			}
			part, err := reader.NextPart()
			if err != nil {
				t.Fatal(err)
			}
			if part.FormName() != "file" || part.FileName() != "fixture.txt" {
				t.Fatalf("part=%s/%s", part.FormName(), part.FileName())
			}
			data, _ := io.ReadAll(part)
			if string(data) != "answer" {
				t.Fatalf("data=%q", data)
			}
			uploaded = true
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"file-1"}`)
		case http.MethodDelete:
			if r.URL.Path != "/v1/files/file-1" || !uploaded {
				t.Fatalf("delete path=%s uploaded=%v", r.URL.Path, uploaded)
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := NewClient(context.Background(), server.URL)
	file, err := client.UploadFile(context.Background(), "fixture.txt", strings.NewReader("answer"))
	if err != nil || file.ID != "file-1" || file.Filename != "fixture.txt" {
		t.Fatalf("file=%#v err=%v", file, err)
	}
	if err := client.DeleteFile(context.Background(), file.ID); err != nil {
		t.Fatal(err)
	}
}
