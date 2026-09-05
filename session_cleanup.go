package induction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SessionCleanupResult reports the session files removed by CleanSessions.
type SessionCleanupResult struct {
	Scanned              int
	Deleted              int
	InvalidDeleted       int
	NullSnapshotsDeleted int
}

// CleanSessions removes invalid session JSON files and valid sessions whose
// snapshots field is explicitly null. Sessions with an empty snapshots array
// are retained.
func CleanSessions(directory string) (SessionCleanupResult, error) {
	if directory == "" {
		directory = DefaultDashboardSessionsDirectory
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return SessionCleanupResult{}, nil
	}
	if err != nil {
		return SessionCleanupResult{}, fmt.Errorf("clean sessions: list directory %q: %w", directory, err)
	}
	var result SessionCleanupResult
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		result.Scanned++
		path := filepath.Join(directory, entry.Name())
		contents, readErr := os.ReadFile(path)
		invalid := readErr != nil
		nullSnapshots := false
		if !invalid {
			var envelope struct {
				Snapshots json.RawMessage `json:"snapshots"`
			}
			if json.Unmarshal(contents, &envelope) != nil || loadErrForCleanup(path) != nil {
				invalid = true
			} else {
				nullSnapshots = bytes.Equal(bytes.TrimSpace(envelope.Snapshots), []byte("null"))
			}
		}
		if !invalid && !nullSnapshots {
			continue
		}
		if err := os.Remove(path); err != nil {
			return result, fmt.Errorf("clean sessions: remove %q: %w", path, err)
		}
		result.Deleted++
		if invalid {
			result.InvalidDeleted++
		} else {
			result.NullSnapshotsDeleted++
		}
	}
	return result, nil
}

func loadErrForCleanup(path string) error {
	_, err := loadChatSessionFromPath(path)
	return err
}
