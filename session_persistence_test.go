package induction

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestChatSessionRoundTripAndRename(t *testing.T) {
	directory := t.TempDir()
	original, err := newChatSession("  Go Interfaces  ", ChatRequest{Model: "model", Messages: []Message{{Role: "user", Content: "Hello"}}})
	if err != nil {
		t.Fatal(err)
	}
	original.Snapshots = []*ModelSnapshot{{ModelID: "model", Messages: []Message{{Role: "user", Content: "Hello"}}, Interaction: []Interaction{{Content: "Hi", ReasoningContent: "thinking", Response: "raw"}}}}
	if err := saveChatSessionToDir(directory, original); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, original.ID+".json")
	loaded, err := loadChatSessionFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "Go Interfaces" || loaded.Model != "model" || len(loaded.Messages) != 1 || len(loaded.Snapshots) != 1 || len(loaded.Snapshots[0].Interaction) != 1 || loaded.Snapshots[0].Interaction[0].Content != "Hi" {
		t.Fatalf("unexpected session: %#v", loaded)
	}
	created := loaded.CreatedAt
	loaded.Title = "Understanding Go Interfaces"
	if err := saveChatSessionToDir(directory, loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.CreatedAt != created {
		t.Fatal("save changed CreatedAt")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("rename created duplicate files: %d", len(entries))
	}
	loaded, err = loadChatSessionFromPath(path)
	if err != nil || loaded.Title != "Understanding Go Interfaces" {
		t.Fatalf("renamed session not persisted: %v %#v", err, loaded)
	}
}

func TestUnsavedChatSessionsAreStoredButNotListed(t *testing.T) {
	directory := t.TempDir()
	unsaved, err := newUnsavedChatSession(ChatRequest{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveChatSessionToDir(directory, unsaved); err != nil {
		t.Fatal(err)
	}
	saved, err := newChatSession("Visible", ChatRequest{Model: "model"})
	if err != nil {
		t.Fatal(err)
	}
	if err := saveChatSessionToDir(directory, saved); err != nil {
		t.Fatal(err)
	}
	loaded, err := loadChatSessionFromPath(filepath.Join(directory, unsaved.ID+".json"))
	if err != nil || loaded.Type != sessionTypeUnsaved {
		t.Fatalf("unsaved session was not persisted distinctly: %v %#v", err, loaded)
	}
	list, err := listChatSessionsFromDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != saved.ID {
		t.Fatalf("unsaved session appeared in load list: %#v", list)
	}
}

func TestChatSessionListingSkipsInvalidAndSorts(t *testing.T) {
	directory := t.TempDir()
	first := &ChatSession{Version: chatSessionVersion, ID: "11111111-1111-4111-8111-111111111111", Title: "Testing", Model: "one", CreatedAt: time.Unix(1, 0).UTC(), UpdatedAt: time.Unix(1, 0).UTC(), Messages: []Message{}}
	second := &ChatSession{Version: chatSessionVersion, ID: "22222222-2222-4222-8222-222222222222", Title: "Testing", Model: "two", CreatedAt: time.Unix(2, 0).UTC(), UpdatedAt: time.Unix(2, 0).UTC(), Messages: []Message{{Role: "user", Content: "x"}}}
	if err := saveChatSessionToDir(directory, first); err != nil {
		t.Fatal(err)
	}
	if err := saveChatSessionToDir(directory, second); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "broken.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	// saveChatSessionToDir updates timestamps, so compare the ordering after save.
	list, err := listChatSessionsFromDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != second.ID || list[1].ID != first.ID {
		t.Fatalf("unexpected list: %#v", list)
	}
}

func TestNormalizeSessionTitle(t *testing.T) {
	if got, err := normalizeSessionTitle("  日本語 conversation  "); err != nil || got != "日本語 conversation" {
		t.Fatalf("unicode title: %q %v", got, err)
	}
	if _, err := normalizeSessionTitle("   "); err == nil {
		t.Fatal("empty title accepted")
	}
	if _, err := normalizeSessionTitle(strings.Repeat("x", maxSessionTitleRunes+1)); err == nil {
		t.Fatal("long title accepted")
	}
}

func TestListChatSessionsMissingDirectory(t *testing.T) {
	list, err := listChatSessionsFromDir(filepath.Join(t.TempDir(), "missing"))
	if err != nil || len(list) != 0 {
		t.Fatalf("missing directory: %v %#v", err, list)
	}
}

func TestCleanupNullSnapshotSessions(t *testing.T) {
	directory := t.TempDir()
	base := `{"version":1,"id":"11111111-1111-4111-8111-111111111111","type":"unsaved","title":"","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z","model":"model","messages":[]}`
	if err := os.WriteFile(filepath.Join(directory, "null.json"), []byte(strings.TrimSuffix(base, "}")+`,"snapshots":null}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "empty.json"), []byte(strings.TrimSuffix(base, "}")+`,"snapshots":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "broken.json"), []byte(`{"snapshots":null`), 0o600); err != nil {
		t.Fatal(err)
	}
	removed, err := cleanupNullSnapshotSessionsFromDir(directory)
	if err != nil || removed != 1 {
		t.Fatalf("cleanup removed=%d err=%v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(directory, "null.json")); !os.IsNotExist(err) {
		t.Fatalf("null snapshot session remains: %v", err)
	}
	for _, name := range []string{"empty.json", "broken.json"} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			t.Fatalf("cleanup removed %s: %v", name, err)
		}
	}
}
