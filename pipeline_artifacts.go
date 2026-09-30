package induction

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// PipelineArtifact is the durable boundary between pipeline steps. Data is
// stored as the exact bytes that were validated and sent to later steps.
type PipelineArtifact struct {
	ID        string    `json:"id"`
	RunID     string    `json:"run_id"`
	StepName  string    `json:"step_name"`
	Name      string    `json:"name"`
	MediaType string    `json:"media_type"`
	SHA256    string    `json:"sha256"`
	CreatedAt time.Time `json:"created_at"`
	Data      []byte    `json:"data"`
}

func persistPipelineArtifact(runID, stepName, name, mediaType string, data []byte) (*PipelineArtifact, error) {
	if runID == "" {
		return nil, fmt.Errorf("pipeline run ID is required")
	}
	clean := filepath.Clean(name)
	if filepath.IsAbs(name) || clean == ".." || len(clean) >= 3 && clean[:3] == ".."+string(filepath.Separator) {
		return nil, fmt.Errorf("invalid artifact name %q", name)
	}
	h := sha256.Sum256(data)
	a := &PipelineArtifact{ID: artifactID(runID, stepName, clean), RunID: runID, StepName: stepName, Name: clean, MediaType: mediaType, SHA256: hex.EncodeToString(h[:]), CreatedAt: time.Now().UTC(), Data: append([]byte(nil), data...)}
	dir := filepath.Join(".pipeline-artifacts", runID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, clean)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".artifact-*")
	if err != nil {
		return nil, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err = tmp.Write(data); err != nil {
		tmp.Close()
		return nil, err
	}
	if err = tmp.Sync(); err != nil {
		tmp.Close()
		return nil, err
	}
	if err = tmp.Close(); err != nil {
		return nil, err
	}
	if existing, err := os.ReadFile(path); err == nil {
		if string(existing) != string(data) {
			return nil, fmt.Errorf("conflicting artifact %q already exists", clean)
		}
		return a, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return nil, err
	}
	return a, nil
}

func loadPipelineArtifact(runID, name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(".pipeline-artifacts", runID, filepath.Clean(name)))
}
func artifactID(runID, step, name string) string {
	h := sha256.Sum256([]byte(runID + "\n" + step + "\n" + name))
	return "artifact_" + hex.EncodeToString(h[:])[:20]
}
