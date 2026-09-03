package induction

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestSnapshotSessionPersistsGrowingSlice(t *testing.T) {
	root := filepath.Join(t.TempDir(), "snapshots")
	session, err := newSnapshotSessionAt(root, "org/model")
	if err != nil {
		t.Fatalf("newSnapshotSessionAt: %v", err)
	}
	if filepath.Base(filepath.Dir(session.path)) != "org_model" {
		t.Fatalf("model directory was not made path-safe: %q", session.path)
	}
	chatID := filepath.Base(session.path)
	if !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\.json$`).MatchString(chatID) {
		t.Fatalf("snapshot filename is not a UUID: %q", chatID)
	}

	snapshots := []*ModelSnapshot{{ModelID: "org/model"}}
	if err := session.save(snapshots); err != nil {
		t.Fatalf("save first snapshot: %v", err)
	}
	snapshots = append(snapshots, &ModelSnapshot{ModelID: "org/model"})
	if err := session.save(snapshots); err != nil {
		t.Fatalf("save second snapshot: %v", err)
	}

	contents, err := os.ReadFile(session.path)
	if err != nil {
		t.Fatalf("read snapshots: %v", err)
	}
	var persisted []*ModelSnapshot
	if err := json.Unmarshal(contents, &persisted); err != nil {
		t.Fatalf("decode snapshots: %v", err)
	}
	if len(persisted) != 2 {
		t.Fatalf("persisted %d snapshots, want 2", len(persisted))
	}

	other, err := newSnapshotSessionAt(root, "org/model")
	if err != nil {
		t.Fatalf("new second session: %v", err)
	}
	if other.path == session.path {
		t.Fatal("snapshot sessions reused a chat ID")
	}
}

func TestSnapshotSessionDoesNotCreateEmptyPersistence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "snapshots")
	session, err := newSnapshotSessionAt(root, "model")
	if err != nil {
		t.Fatalf("newSnapshotSessionAt: %v", err)
	}
	if _, err := os.Stat(session.path); !os.IsNotExist(err) {
		t.Fatalf("session creation persisted an empty file: %v", err)
	}
	if err := session.save(nil); err != nil {
		t.Fatalf("save empty snapshots: %v", err)
	}
	if _, err := os.Stat(session.path); !os.IsNotExist(err) {
		t.Fatalf("empty save persisted a file: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(session.path)); !os.IsNotExist(err) {
		t.Fatalf("empty session created a model directory: %v", err)
	}
}
