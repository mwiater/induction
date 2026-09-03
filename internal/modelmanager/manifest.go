// Package modelmanager discovers, downloads, verifies, and updates local
// model artifacts obtained from Hugging Face.
package modelmanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manifest records immutable metadata and integrity information for an
// installed model artifact or shard set.
type Manifest struct {
	SchemaVersion int            `json:"schemaVersion"`
	ModelFile     string         `json:"modelFile,omitempty"`
	RepositoryID  string         `json:"repositoryId"`
	Revision      string         `json:"revision"`
	DownloadURL   string         `json:"downloadUrl,omitempty"`
	DownloadedAt  time.Time      `json:"downloadedAt"`
	SizeBytes     int64          `json:"sizeBytes,omitempty"`
	ETag          string         `json:"etag,omitempty"`
	LFSOID        string         `json:"lfsOid,omitempty"`
	SHA256        string         `json:"sha256,omitempty"`
	Files         []ManifestFile `json:"files,omitempty"`
}

// ManifestFile records metadata for one file in a multi-file model install.
type ManifestFile struct {
	ModelFile   string `json:"modelFile"`
	DownloadURL string `json:"downloadUrl"`
	SizeBytes   int64  `json:"sizeBytes"`
	SHA256      string `json:"sha256"`
	ETag        string `json:"etag,omitempty"`
	LFSOID      string `json:"lfsOid,omitempty"`
}

// WriteManifestAtomic writes manifest through a synced temporary file and
// atomic rename so an interrupted write cannot leave a partial manifest.
func WriteManifestAtomic(path string, manifest Manifest) error {
	if manifest.SchemaVersion == 0 {
		manifest.SchemaVersion = 1
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temp, err := os.CreateTemp(filepath.Dir(path), ".manifest-*.tmp")
	if err != nil {
		return err
	}
	name := temp.Name()
	ok := false
	defer func() {
		if !ok {
			_ = os.Remove(name)
		}
	}()
	if _, err = temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err = temp.Sync(); err != nil {
		_ = temp.Close()
		return err
	}
	if err = temp.Close(); err != nil {
		return err
	}
	if err = os.Rename(name, path); err != nil {
		return err
	}
	ok = true
	dir, err := os.Open(filepath.Dir(path))
	if err == nil {
		err = dir.Sync()
		_ = dir.Close()
	}
	if err != nil {
		return fmt.Errorf("sync manifest directory: %w", err)
	}
	return nil
}
