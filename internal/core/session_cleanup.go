// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// FailedSession identifies a persisted session with a recorded failure.
type FailedSession struct {
	Filename string
	Error    string
}

// SessionCleanupResult reports the session files removed by CleanSessions.
// SessionCleanupResult is an exported type in this package's public data model.
type SessionCleanupResult struct {
	// Scanned stores the value represented by this member.
	Scanned int
	// Deleted stores the value represented by this member.
	Deleted int
	// InvalidDeleted stores the value represented by this member.
	InvalidDeleted int
	// NullSnapshotsDeleted stores the value represented by this member.
	NullSnapshotsDeleted int
	// EmptyDeleted stores the value represented by this member.
	EmptyDeleted int
	// FailedDeleted stores the value represented by this member.
	FailedDeleted int
}

// InferenceDataPurgeResult reports the inference-data directories emptied by
// PurgeInferenceData.
// InferenceDataPurgeResult is an exported type in this package's public data model.
type InferenceDataPurgeResult struct {
	// Directories stores the value represented by this member.
	Directories int
	// Entries stores the value represented by this member.
	Entries int
}

// inferenceDataDirectories holds package-level state used internally by this package.
var inferenceDataDirectories = []string{".sessions", ".pipeline-artifacts", ".batches"}

// PurgeInferenceData removes all contents of the persisted inference-data
// directories while preserving the directories themselves. Missing
// directories are ignored.
func PurgeInferenceData() (InferenceDataPurgeResult, error) {
	return purgeInferenceDataFromRoot(".")
}

// purgeInferenceDataFromRoot performs the internal operation described by its signature.
func purgeInferenceDataFromRoot(root string) (InferenceDataPurgeResult, error) {
	var result InferenceDataPurgeResult
	for _, name := range inferenceDataDirectories {
		directory := filepath.Join(root, name)
		entries, err := os.ReadDir(directory)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, fmt.Errorf("purge inference data: list directory %q: %w", directory, err)
		}
		result.Directories++
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(directory, entry.Name())); err != nil {
				return result, fmt.Errorf("purge inference data: remove %q: %w", filepath.Join(directory, entry.Name()), err)
			}
			result.Entries++
		}
	}
	return result, nil
}

// CleanSessions removes invalid session JSON files and valid sessions whose
// snapshots field is explicitly null, whose snapshots array is empty, or that
// contain a recorded error.
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
		emptySnapshots := false
		failed := false
		if !invalid {
			var envelope struct {
				Snapshots json.RawMessage `json:"snapshots"`
				Error     string          `json:"error"`
			}
			if json.Unmarshal(contents, &envelope) != nil || loadErrForCleanup(path) != nil {
				invalid = true
			} else {
				nullSnapshots = bytes.Equal(bytes.TrimSpace(envelope.Snapshots), []byte("null"))
				emptySnapshots = bytes.Equal(bytes.TrimSpace(envelope.Snapshots), []byte("[]"))
				failed = strings.TrimSpace(envelope.Error) != ""
			}
		}
		if !invalid && !nullSnapshots && !emptySnapshots && !failed {
			continue
		}
		if err := os.Remove(path); err != nil {
			return result, fmt.Errorf("clean sessions: remove %q: %w", path, err)
		}
		result.Deleted++
		if invalid {
			result.InvalidDeleted++
		} else {
			switch {
			case failed:
				result.FailedDeleted++
			case nullSnapshots:
				result.NullSnapshotsDeleted++
			case emptySnapshots:
				result.EmptyDeleted++
			}
		}
	}
	return result, nil
}

// ListFailedSessions returns persisted session files with a non-empty error
// field. Invalid JSON files are ignored because they are not failed sessions.
func ListFailedSessions(directory string) ([]FailedSession, error) {
	if directory == "" {
		directory = DefaultDashboardSessionsDirectory
	}
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []FailedSession{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list failed sessions: list directory %q: %w", directory, err)
	}

	result := make([]FailedSession, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var envelope struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(contents, &envelope); err != nil || strings.TrimSpace(envelope.Error) == "" {
			continue
		}
		result = append(result, FailedSession{Filename: path, Error: envelope.Error})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Filename < result[j].Filename })
	return result, nil
}

// loadErrForCleanup performs the internal operation described by its signature.
func loadErrForCleanup(path string) error {
	_, err := loadChatSessionFromPath(path)
	return err
}
