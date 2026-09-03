package modelmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Destination contains the confined paths used for one repository artifact.
type Destination struct {
	Directory string
	Artifact  string
	Manifest  string
}

// ResolveDestination maps a repository-relative filename into modelsPath after
// rejecting traversal and absolute-path escapes.
func ResolveDestination(modelsPath, repository, filename string) (Destination, error) {
	repo := strings.Split(repository, "/")
	if len(repo) != 2 || !safeComponent(repo[0]) || !safeComponent(repo[1]) {
		return Destination{}, fmt.Errorf("repository ID must contain exactly two safe components")
	}
	if filepath.IsAbs(filename) || filename == "" {
		return Destination{}, fmt.Errorf("artifact path must be repository-relative")
	}
	clean := filepath.Clean(filepath.FromSlash(filename))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return Destination{}, fmt.Errorf("unsafe artifact path %q", filename)
	}
	base, err := filepath.Abs(modelsPath)
	if err != nil {
		return Destination{}, err
	}
	dir := filepath.Join(base, repo[0], repo[1])
	artifact := filepath.Join(dir, clean)
	if err := confined(base, artifact); err != nil {
		return Destination{}, err
	}
	return Destination{Directory: dir, Artifact: artifact, Manifest: artifact + ".json"}, nil
}

func safeComponent(value string) bool {
	return value != "" && value != "." && value != ".." && !strings.ContainsAny(value, "/\\\x00")
}
func confined(base, target string) error {
	rel, err := filepath.Rel(base, target)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path escapes ModelsPath")
	}
	return nil
}

// DiskPreflight reports whether enough space is available for an installation.
type DiskPreflight struct {
	AvailableBytes uint64 `json:"availableBytes"`
	RequiredBytes  uint64 `json:"requiredBytes"`
	SizeKnown      bool   `json:"sizeKnown"`
	Sufficient     bool   `json:"sufficient"`
}

// CheckDisk creates path if needed and estimates the space required for a download.
func CheckDisk(path string, size int64) (DiskPreflight, error) {
	if err := os.MkdirAll(path, 0o755); err != nil {
		return DiskPreflight{}, err
	}
	var stat unix.Statfs_t
	if err := unix.Statfs(path, &stat); err != nil {
		return DiskPreflight{}, err
	}
	available := stat.Bavail * uint64(stat.Bsize)
	if size <= 0 {
		return DiskPreflight{AvailableBytes: available, SizeKnown: false, Sufficient: true}, nil
	}
	required := uint64(size)*2 + uint64(size)/10
	return DiskPreflight{AvailableBytes: available, RequiredBytes: required, SizeKnown: true, Sufficient: available >= required}, nil
}
