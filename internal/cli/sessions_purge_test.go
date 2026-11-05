package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPurgeLocalData(t *testing.T) {
	root := t.TempDir()
	for _, name := range purgeDataDirectories {
		path := filepath.Join(root, name, "nested")
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "data.json"), []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	keep := filepath.Join(root, "keep.txt")
	if err := os.WriteFile(keep, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := purgeLocalData(root); err != nil {
		t.Fatal(err)
	}
	for _, name := range purgeDataDirectories {
		if _, err := os.Lstat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("%s was not removed: %v", name, err)
		}
	}
	if _, err := os.ReadFile(keep); err != nil {
		t.Fatalf("unrelated file was affected: %v", err)
	}
	if err := purgeLocalData(root); err != nil {
		t.Fatalf("purging missing directories failed: %v", err)
	}
}

func TestPurgeLocalDataRejectsSymlinkBeforeDeletion(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ".batches")); err != nil {
		t.Fatal(err)
	}
	if err := purgeLocalData(root); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("expected symlink rejection, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".sessions")); err != nil {
		t.Fatalf("preflight deleted a directory: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("symlink target was affected: %v", err)
	}
}

func TestSessionsPurgeCommandRegistered(t *testing.T) {
	root := NewRootCommand()
	command, _, err := root.Find([]string{"sessions", "purge"})
	if err != nil || command == nil || command.CommandPath() != "induction sessions purge" {
		t.Fatalf("purge command unavailable: %v %#v", err, command)
	}
}

func TestSessionsPurgeCommandUsesCurrentDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".sessions"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	command := NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"sessions", "purge"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, ".sessions")); !os.IsNotExist(err) {
		t.Fatalf("session directory remains: %v", err)
	}
	if !strings.Contains(output.String(), "Purged .sessions/") {
		t.Fatalf("missing completion message: %q", output.String())
	}
}
