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
// Manifest is an exported type in this package's public data model.
type Manifest struct {
	// SchemaVersion stores the value represented by this member.
	SchemaVersion int `json:"schemaVersion"`
	// ModelFile stores the value represented by this member.
	ModelFile string `json:"modelFile,omitempty"`
	// RepositoryID stores the value represented by this member.
	RepositoryID string `json:"repositoryId"`
	// Revision stores the value represented by this member.
	Revision string `json:"revision"`
	// DownloadURL stores the value represented by this member.
	DownloadURL string `json:"downloadUrl,omitempty"`
	// DownloadedAt stores the value represented by this member.
	DownloadedAt time.Time `json:"downloadedAt"`
	// SizeBytes stores the value represented by this member.
	SizeBytes int64 `json:"sizeBytes,omitempty"`
	// ETag stores the value represented by this member.
	ETag string `json:"etag,omitempty"`
	// LFSOID stores the value represented by this member.
	LFSOID string `json:"lfsOid,omitempty"`
	// SHA256 stores the value represented by this member.
	SHA256 string `json:"sha256,omitempty"`
	// Files stores the value represented by this member.
	Files []ManifestFile `json:"files,omitempty"`
}

// ManifestFile records metadata for one file in a multi-file model install.
// ManifestFile is an exported type in this package's public data model.
type ManifestFile struct {
	// ModelFile stores the value represented by this member.
	ModelFile string `json:"modelFile"`
	// DownloadURL stores the value represented by this member.
	DownloadURL string `json:"downloadUrl"`
	// SizeBytes stores the value represented by this member.
	SizeBytes int64 `json:"sizeBytes"`
	// SHA256 stores the value represented by this member.
	SHA256 string `json:"sha256"`
	// ETag stores the value represented by this member.
	ETag string `json:"etag,omitempty"`
	// LFSOID stores the value represented by this member.
	LFSOID string `json:"lfsOid,omitempty"`
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
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
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
