package modelmanager

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

// TransferPhase identifies the stage represented by a ProgressUpdate.
type TransferPhase string

const (
	// PhaseDownloading indicates that artifact bytes are being transferred.
	PhaseDownloading TransferPhase = "downloading"
	// PhaseValidating indicates that a downloaded artifact is being hashed.
	PhaseValidating TransferPhase = "validating"
)

// ProgressUpdate reports transfer progress for one artifact.
type ProgressUpdate struct {
	Phase                      TransferPhase
	CompletedBytes, TotalBytes int64
}

// ProgressFunc receives download progress updates.
type ProgressFunc func(ProgressUpdate)

// DownloadHTTP streams one immutable Hub artifact into a same-directory
// staging file, reports byte progress, validates it, and atomically installs
// the artifact before writing its provenance manifest.
func DownloadHTTP(ctx context.Context, client *http.Client, request DownloadRequest, progress ProgressFunc) (Manifest, error) {
	destination, err := ResolveDestination(request.ModelsPath, request.Repository, request.File)
	if err != nil {
		return Manifest{}, err
	}
	if !request.Overwrite {
		if fileExists(destination.Artifact) || fileExists(destination.Manifest) {
			return Manifest{}, fmt.Errorf("artifact or manifest already exists; explicit overwrite approval is required")
		}
	}
	preflight, err := CheckDisk(destination.Directory, request.Size)
	if err != nil {
		return Manifest{}, err
	}
	if !preflight.Sufficient {
		return Manifest{}, fmt.Errorf("insufficient disk space: need %d bytes, have %d", preflight.RequiredBytes, preflight.AvailableBytes)
	}
	if err = os.MkdirAll(filepath.Dir(destination.Artifact), 0o755); err != nil {
		return Manifest{}, err
	}
	url, err := DownloadURL(request.Repository, request.Revision, request.File)
	if err != nil {
		return Manifest{}, err
	}
	staging := destination.Artifact + ".part"
	completed, err := streamHubFile(ctx, client, url, staging, request.Size, progress)
	if err != nil {
		return Manifest{}, err
	}
	if request.Size > 0 && completed != request.Size {
		return Manifest{}, fmt.Errorf("download size mismatch: expected %d bytes, received %d", request.Size, completed)
	}
	hash, err := hashFileProgress(ctx, staging, completed, progress)
	if err != nil {
		return Manifest{}, err
	}
	if err = os.Rename(staging, destination.Artifact); err != nil {
		return Manifest{}, fmt.Errorf("install downloaded artifact: %w", err)
	}
	manifest := Manifest{SchemaVersion: 1, ModelFile: filepath.ToSlash(request.File), RepositoryID: request.Repository, Revision: request.Revision, DownloadURL: url, DownloadedAt: time.Now().UTC(), SizeBytes: completed, ETag: request.ETag, LFSOID: request.LFSOID, SHA256: hash}
	if err = WriteManifestAtomic(destination.Manifest, manifest); err != nil {
		return Manifest{}, fmt.Errorf("model downloaded but manifest is incomplete: %w", err)
	}
	return manifest, nil
}

// DownloadMultiHTTP applies the same measured transfer and validation path to
// every shard and then writes one schema-v2 logical-installation manifest.
func DownloadMultiHTTP(ctx context.Context, client *http.Client, modelsPath, repository, revision string, files []ModelFile, overwrite bool, progress ProgressFunc) (Manifest, string, error) {
	if len(files) < 2 {
		return Manifest{}, "", fmt.Errorf("multiple shards are required")
	}
	stem, set, err := ShardSet(files[0], files)
	if err != nil {
		return Manifest{}, "", err
	}
	logical, err := ResolveDestination(modelsPath, repository, stem)
	if err != nil {
		return Manifest{}, "", err
	}
	var aggregate int64
	for _, file := range set {
		aggregate += file.Size
	}
	preflight, err := CheckDisk(logical.Directory, aggregate)
	if err != nil {
		return Manifest{}, "", err
	}
	if !preflight.Sufficient {
		return Manifest{}, "", fmt.Errorf("insufficient disk space for shard set: need %d bytes, have %d", preflight.RequiredBytes, preflight.AvailableBytes)
	}
	manifest := Manifest{SchemaVersion: 2, RepositoryID: repository, Revision: revision, DownloadedAt: time.Now().UTC()}
	singleManifests := make([]string, 0, len(set))
	for _, file := range set {
		installed, err := DownloadHTTP(ctx, client, DownloadRequest{Repository: repository, File: file.Path, Revision: revision, ModelsPath: modelsPath, Size: file.Size, ETag: file.ETag, LFSOID: file.LFSOID, Overwrite: overwrite}, progress)
		if err != nil {
			return Manifest{}, "", err
		}
		manifest.Files = append(manifest.Files, ManifestFile{ModelFile: installed.ModelFile, DownloadURL: installed.DownloadURL, SizeBytes: installed.SizeBytes, SHA256: installed.SHA256, ETag: installed.ETag, LFSOID: installed.LFSOID})
		destination, _ := ResolveDestination(modelsPath, repository, file.Path)
		singleManifests = append(singleManifests, destination.Manifest)
	}
	manifestPath := logical.Artifact + ".json"
	if err = WriteManifestAtomic(manifestPath, manifest); err != nil {
		return Manifest{}, "", err
	}
	for _, path := range singleManifests {
		if path != manifestPath {
			_ = os.Remove(path)
		}
	}
	return manifest, manifestPath, nil
}

func streamHubFile(ctx context.Context, client *http.Client, url, staging string, expected int64, progress ProgressFunc) (int64, error) {
	if client == nil {
		client = http.DefaultClient
	}
	existing := int64(0)
	if info, err := os.Stat(staging); err == nil {
		existing = info.Size()
		if expected > 0 && existing > expected {
			existing = 0
		}
	}
	if expected > 0 && existing == expected {
		reportProgress(progress, PhaseDownloading, existing, expected)
		return existing, nil
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	if token := os.Getenv("HF_TOKEN"); token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if existing > 0 {
		request.Header.Set("Range", "bytes="+strconv.FormatInt(existing, 10)+"-")
	}
	response, err := client.Do(request)
	if err != nil {
		return existing, fmt.Errorf("download model artifact: %w", err)
	}
	defer response.Body.Close()
	appendMode := existing > 0 && response.StatusCode == http.StatusPartialContent
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		return existing, fmt.Errorf("download model artifact: Hub returned %s", response.Status)
	}
	flags := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
		existing = 0
	}
	file, err := os.OpenFile(staging, flags, 0o644)
	if err != nil {
		return existing, err
	}
	defer file.Close()
	total := expected
	if total <= 0 && response.ContentLength >= 0 {
		total = existing + response.ContentLength
	}
	done := existing
	reportProgress(progress, PhaseDownloading, done, total)
	buffer := make([]byte, 1024*1024)
	last := time.Now()
	for {
		if err = ctx.Err(); err != nil {
			return done, err
		}
		n, readErr := response.Body.Read(buffer)
		if n > 0 {
			if _, err = file.Write(buffer[:n]); err != nil {
				return done, err
			}
			done += int64(n)
			if time.Since(last) >= 100*time.Millisecond {
				reportProgress(progress, PhaseDownloading, done, total)
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return done, readErr
		}
	}
	if err = file.Sync(); err != nil {
		return done, err
	}
	reportProgress(progress, PhaseDownloading, done, total)
	return done, nil
}

func hashFileProgress(ctx context.Context, path string, total int64, progress ProgressFunc) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	buffer := make([]byte, 1024*1024)
	done := int64(0)
	reportProgress(progress, PhaseValidating, 0, total)
	last := time.Now()
	for {
		if err = ctx.Err(); err != nil {
			return "", err
		}
		n, readErr := file.Read(buffer)
		if n > 0 {
			_, _ = hash.Write(buffer[:n])
			done += int64(n)
			if time.Since(last) >= 100*time.Millisecond {
				reportProgress(progress, PhaseValidating, done, total)
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", readErr
		}
	}
	reportProgress(progress, PhaseValidating, done, total)
	return hex.EncodeToString(hash.Sum(nil)), nil
}
func reportProgress(callback ProgressFunc, phase TransferPhase, done, total int64) {
	if callback != nil {
		callback(ProgressUpdate{phase, done, total})
	}
}
