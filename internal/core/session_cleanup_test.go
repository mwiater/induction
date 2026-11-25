package induction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanSessionsRemovesInvalidEmptyNullAndFailedSessions(t *testing.T) {
	directory := t.TempDir()
	valid := &ChatSession{Version: chatSessionVersion, ID: "00000000-0000-0000-0000-000000000101", Type: sessionTypeDirect, Model: "model", Messages: []Message{}, Snapshots: []*ModelSnapshot{{ModelID: "model"}}}
	if err := saveChatSessionToDir(directory, valid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "invalid.json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	empty := filepath.Join(directory, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"version":1,"id":"00000000-0000-0000-0000-000000000103","type":"direct","saved":false,"title":"","model":"model","messages":[],"snapshots":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	failed := filepath.Join(directory, "failed.json")
	if err := os.WriteFile(failed, []byte(`{"version":1,"id":"00000000-0000-0000-0000-000000000104","type":"direct","saved":false,"title":"","model":"model","messages":[],"snapshots":[],"error":"model failed"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	null := filepath.Join(directory, "null.json")
	if err := os.WriteFile(null, []byte(`{"version":1,"id":"00000000-0000-0000-0000-000000000102","type":"direct","saved":false,"title":"","model":"model","messages":[],"snapshots":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CleanSessions(directory)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 5 || result.Deleted != 4 || result.InvalidDeleted != 1 || result.NullSnapshotsDeleted != 1 || result.EmptyDeleted != 1 || result.FailedDeleted != 1 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(directory, valid.ID+".json")); err != nil {
		t.Fatalf("valid session was removed: %v", err)
	}
}

func TestListFailedSessions(t *testing.T) {
	directory := t.TempDir()
	if err := os.WriteFile(filepath.Join(directory, "b-failed.json"), []byte(`{"error":"second failure"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "a-failed.json"), []byte(`{"error":"first failure"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "ok.json"), []byte(`{"error":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "invalid.json"), []byte(`not json`), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ListFailedSessions(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("got %d failed sessions, want 2: %+v", len(result), result)
	}
	if result[0].Filename != filepath.Join(directory, "a-failed.json") || result[0].Error != "first failure" {
		t.Fatalf("unexpected first result: %+v", result[0])
	}
	if result[1].Filename != filepath.Join(directory, "b-failed.json") || result[1].Error != "second failure" {
		t.Fatalf("unexpected second result: %+v", result[1])
	}
}
