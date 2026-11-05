package induction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SessionCleanupResult reports the session files removed by CleanSessions.
type SessionCleanupResult struct {
	Scanned               int
	Deleted               int
	InvalidDeleted        int
	NullSnapshotsDeleted  int
	ActiveOrRecentSkipped int
}

// CleanSessions removes old invalid JSON and terminal pipeline sessions with
// explicitly null snapshots. Running and ambiguous sessions are preserved.
// Session writes use temporary files and atomic rename; the age guard also
// protects files written in place by older or external clients.
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
		if !entry.Type().IsRegular() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		result.Scanned++
		path := filepath.Join(directory, entry.Name())
		before, err := os.Lstat(path)
		if err != nil || !before.Mode().IsRegular() {
			result.ActiveOrRecentSkipped++
			continue
		}
		contents, readErr := os.ReadFile(path)
		if readErr != nil {
			return result, fmt.Errorf("clean sessions: read %q: %w", path, readErr)
		}
		after, statErr := os.Lstat(path)
		if statErr != nil || !after.Mode().IsRegular() || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) {
			result.ActiveOrRecentSkipped++
			continue
		}
		invalid := false
		nullSnapshots := false
		if !invalid {
			var envelope struct {
				Type           string          `json:"type"`
				PipelineStatus string          `json:"pipeline_status"`
				Snapshots      json.RawMessage `json:"snapshots"`
			}
			var session ChatSession
			if json.Unmarshal(contents, &envelope) == nil && envelope.Type == sessionTypePipeline && (envelope.PipelineStatus == "running" || envelope.PipelineStatus == "") {
				result.ActiveOrRecentSkipped++
				continue
			}
			if json.Unmarshal(contents, &envelope) != nil || json.Unmarshal(contents, &session) != nil || validateChatSession(&session) != nil {
				invalid = true
			} else {
				nullSnapshots = bytes.Equal(bytes.TrimSpace(envelope.Snapshots), []byte("null"))
				if nullSnapshots && (session.Type != sessionTypePipeline || session.PipelineStatus != "completed" && session.PipelineStatus != "failed") {
					result.ActiveOrRecentSkipped++
					continue
				}
			}
		}
		if !invalid && !nullSnapshots {
			continue
		}
		if time.Since(after.ModTime()) < time.Minute {
			result.ActiveOrRecentSkipped++
			continue
		}
		// Recheck before unlinking. Another writer may have replaced the file
		// while the cleaner inspected its contents.
		latest, err := os.Lstat(path)
		if err != nil || !latest.Mode().IsRegular() || !latest.ModTime().Equal(after.ModTime()) || latest.Size() != after.Size() {
			result.ActiveOrRecentSkipped++
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
