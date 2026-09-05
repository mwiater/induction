package induction

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanSessionsRemovesInvalidAndNullSnapshotsOnly(t *testing.T) {
	directory := t.TempDir()
	valid := &ChatSession{Version: chatSessionVersion, ID: "00000000-0000-0000-0000-000000000101", Type: sessionTypeUnsaved, Model: "model", Messages: []Message{}, Snapshots: []*ModelSnapshot{}}
	if err := saveChatSessionToDir(directory, valid); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "invalid.json"), []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	null := filepath.Join(directory, "null.json")
	if err := os.WriteFile(null, []byte(`{"version":1,"id":"00000000-0000-0000-0000-000000000102","type":"unsaved","title":"","model":"model","messages":[],"snapshots":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CleanSessions(directory)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 3 || result.Deleted != 2 || result.InvalidDeleted != 1 || result.NullSnapshotsDeleted != 1 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(directory, valid.ID+".json")); err != nil {
		t.Fatalf("valid empty-snapshot session was removed: %v", err)
	}
}
