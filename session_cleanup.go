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
	EmptyDeleted         int
	FailedDeleted        int
}

// InferenceDataPurgeResult reports the inference-data directories emptied by
// PurgeInferenceData.
type InferenceDataPurgeResult struct {
	Directories int
	Entries     int
}

var inferenceDataDirectories = []string{".sessions", ".pipeline-artifacts", ".batches"}

// PurgeInferenceData removes all contents of the persisted inference-data
// directories while preserving the directories themselves. Missing
// directories are ignored.
func PurgeInferenceData() (InferenceDataPurgeResult, error) {
	return purgeInferenceDataFromRoot(".")
}

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

func loadErrForCleanup(path string) error {
	_, err := loadChatSessionFromPath(path)
	return err
}
