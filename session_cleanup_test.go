package induction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCleanSessionsKeepsActiveSessionsAndRemovesOldInvalidData(t *testing.T) {
	directory := t.TempDir()
	valid := &ChatSession{Version: chatSessionVersion, ID: "00000000-0000-0000-0000-000000000101", Type: sessionTypeDirect, Model: "model", Messages: []Message{}, Snapshots: []*ModelSnapshot{}}
	if err := saveChatSessionToDir(directory, valid); err != nil {
		t.Fatal(err)
	}
	invalidPath := filepath.Join(directory, "invalid.json")
	if err := os.WriteFile(invalidPath, []byte("not json\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	provenance := &SessionPipeline{Name: "example", Source: "file", Path: "/tmp/example.yaml", SHA256: strings.Repeat("a", 64)}
	terminal := &ChatSession{Version: chatSessionVersion, ID: "00000000-0000-0000-0000-000000000102", Type: sessionTypePipeline, Title: "example", Model: "model", Pipeline: provenance, PipelineStatus: "completed", CompletedAt: &now}
	terminalData, err := json.Marshal(terminal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, terminal.ID+".json"), terminalData, 0o600); err != nil {
		t.Fatal(err)
	}
	running := &ChatSession{Version: chatSessionVersion, ID: "00000000-0000-0000-0000-000000000103", Type: sessionTypePipeline, Title: "example", Model: "model", Pipeline: provenance, PipelineStatus: "running"}
	runningData, err := json.Marshal(running)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, running.ID+".json"), runningData, 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Minute)
	for _, path := range []string{invalidPath, filepath.Join(directory, terminal.ID+".json"), filepath.Join(directory, running.ID+".json")} {
		if err := os.Chtimes(path, old, old); err != nil {
			t.Fatal(err)
		}
	}
	result, err := CleanSessions(directory)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 4 || result.Deleted != 2 || result.InvalidDeleted != 1 || result.NullSnapshotsDeleted != 1 || result.ActiveOrRecentSkipped != 1 {
		t.Fatalf("unexpected cleanup result: %+v", result)
	}
	if _, err := os.Stat(filepath.Join(directory, valid.ID+".json")); err != nil {
		t.Fatalf("valid empty-snapshot session was removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, running.ID+".json")); err != nil {
		t.Fatalf("running pipeline session was removed: %v", err)
	}
}

func TestCleanSessionsDefersRecentInvalidFiles(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "partial.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := CleanSessions(directory)
	if err != nil {
		t.Fatal(err)
	}
	if result.Deleted != 0 || result.ActiveOrRecentSkipped != 1 {
		t.Fatalf("recent file was not protected: %+v", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
