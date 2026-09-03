package induction

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const snapshotDirectory = "snapshots"

type snapshotSession struct {
	path string
}

func newSnapshotSession(modelID string) (*snapshotSession, error) {
	return newSnapshotSessionAt(snapshotDirectory, modelID)
}

func newSnapshotSessionAt(root, modelID string) (*snapshotSession, error) {
	directory := filepath.Join(root, snapshotModelDirectory(modelID))
	for attempt := 0; attempt < 128; attempt++ {
		chatID, err := newUUID()
		if err != nil {
			return nil, fmt.Errorf("generate snapshot chat ID: %w", err)
		}
		path := filepath.Join(directory, chatID+".json")
		if _, err := os.Stat(path); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("check snapshot path: %w", err)
		}
		return &snapshotSession{path: path}, nil
	}
	return nil, fmt.Errorf("could not reserve a unique snapshot chat ID")
}

func (s *snapshotSession) save(snapshots []*ModelSnapshot) error {
	if len(snapshots) == 0 {
		return nil
	}
	contents, err := json.MarshalIndent(snapshots, "", "  ")
	if err != nil {
		return fmt.Errorf("encode snapshots: %w", err)
	}
	contents = append(contents, '\n')

	directory := filepath.Dir(s.path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create snapshot directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".snapshots-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary snapshot file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set snapshot permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write snapshots: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync snapshots: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close snapshots: %w", err)
	}
	if err := os.Rename(temporaryPath, s.path); err != nil {
		return fmt.Errorf("replace snapshot file: %w", err)
	}
	return nil
}

func snapshotModelDirectory(modelID string) string {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		return "unknown-model"
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, modelID)
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[0:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:32], nil
}
