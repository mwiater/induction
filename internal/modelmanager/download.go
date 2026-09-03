package modelmanager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// DownloadRequest specifies one Hugging Face artifact download.
type DownloadRequest struct {
	Repository, File, Revision, ModelsPath string
	Size                                   int64
	ETag, LFSOID                           string
	Overwrite                              bool
}

// Download retrieves, verifies, and records one model artifact.
func Download(ctx context.Context, hfPath string, request DownloadRequest) (Manifest, error) {
	destination, err := ResolveDestination(request.ModelsPath, request.Repository, request.File)
	if err != nil {
		return Manifest{}, err
	}
	if !request.Overwrite {
		if _, err := os.Stat(destination.Artifact); err == nil {
			return Manifest{}, fmt.Errorf("artifact already exists; explicit overwrite approval is required")
		}
		if _, err := os.Stat(destination.Manifest); err == nil {
			return Manifest{}, fmt.Errorf("manifest already exists; explicit overwrite approval is required")
		}
	}
	preflight, err := CheckDisk(destination.Directory, request.Size)
	if err != nil {
		return Manifest{}, err
	}
	if !preflight.Sufficient {
		return Manifest{}, fmt.Errorf("insufficient disk space: need %d bytes, have %d", preflight.RequiredBytes, preflight.AvailableBytes)
	}
	if err := os.MkdirAll(destination.Directory, 0o755); err != nil {
		return Manifest{}, err
	}
	cmd := exec.CommandContext(ctx, hfPath, "download", request.Repository, filepath.ToSlash(request.File), "--revision", request.Revision, "--local-dir", destination.Directory)
	cmd.Env = os.Environ()
	var stderr bytes.Buffer
	cmd.Stdout = &stderr
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return Manifest{}, ctx.Err()
		}
		return Manifest{}, fmt.Errorf("hf download failed: %s", sanitizeDiagnostic(stderr.String()))
	}
	info, err := os.Stat(destination.Artifact)
	if err != nil {
		return Manifest{}, fmt.Errorf("download completed but artifact is missing: %w", err)
	}
	downloadURL, err := DownloadURL(request.Repository, request.Revision, request.File)
	if err != nil {
		return Manifest{}, err
	}
	size := request.Size
	if size <= 0 {
		size = info.Size()
	}
	hash, err := HashFile(ctx, destination.Artifact)
	if err != nil {
		return Manifest{}, fmt.Errorf("model downloaded but verification failed: %w", err)
	}
	manifest := Manifest{SchemaVersion: 1, ModelFile: filepath.ToSlash(request.File), RepositoryID: request.Repository, Revision: request.Revision, DownloadURL: downloadURL, DownloadedAt: time.Now().UTC(), SizeBytes: size, ETag: request.ETag, LFSOID: request.LFSOID, SHA256: hash}
	if err := WriteManifestAtomic(destination.Manifest, manifest); err != nil {
		return Manifest{}, fmt.Errorf("model downloaded but manifest is incomplete: %w", err)
	}
	return manifest, nil
}
