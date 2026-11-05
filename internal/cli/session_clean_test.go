package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionsCleanCommandPreservesRunningPipeline(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".sessions")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "running.json")
	data := `{"version":1,"id":"11111111-1111-4111-8111-111111111111","type":"pipeline","title":"example","model":"local","pipeline_status":"running","snapshots":null}`
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	command := NewRootCommand()
	command.SetArgs([]string{"sessions", "clean"})
	var output bytes.Buffer
	command.SetOut(&output)
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("running session was removed: %v", err)
	}
	if !strings.Contains(output.String(), "Active or recent sessions skipped: 1") {
		t.Fatalf("missing skip count: %s", output.String())
	}
}
