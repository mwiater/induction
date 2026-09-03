package modelmanager

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var shardPattern = regexp.MustCompile(`(?i)^(.*)-([0-9]{5})-of-([0-9]{5})(\.gguf)$`)

// ShardSet returns the common stem and complete shard set for selected. A
// non-sharded file is returned as a one-file set.
func ShardSet(selected ModelFile, files []ModelFile) (string, []ModelFile, error) {
	match := shardPattern.FindStringSubmatch(filepath.Base(selected.Path))
	if match == nil {
		return "", []ModelFile{selected}, nil
	}
	total, _ := strconv.Atoi(match[3])
	result := make([]ModelFile, 0, total)
	seen := map[int]bool{}
	for _, file := range files {
		if filepath.Dir(file.Path) != filepath.Dir(selected.Path) {
			continue
		}
		candidate := shardPattern.FindStringSubmatch(filepath.Base(file.Path))
		if candidate == nil || !strings.EqualFold(candidate[1], match[1]) || candidate[3] != match[3] {
			continue
		}
		part, _ := strconv.Atoi(candidate[2])
		if part < 1 || part > total || seen[part] {
			return "", nil, fmt.Errorf("duplicate or invalid shard number")
		}
		seen[part] = true
		result = append(result, file)
	}
	if len(result) != total {
		return "", nil, fmt.Errorf("incomplete shard set: found %d of %d", len(result), total)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	stem := match[1] + match[4]
	if directory := filepath.Dir(selected.Path); directory != "." {
		stem = filepath.ToSlash(filepath.Join(directory, stem))
	}
	return stem, result, nil
}

// DownloadMulti downloads and verifies a selected model file or complete shard set.
func DownloadMulti(ctx context.Context, hfPath, modelsPath, repository, revision string, files []ModelFile) (Manifest, string, error) {
	if len(files) == 0 {
		return Manifest{}, "", fmt.Errorf("no files selected")
	}
	stem, set, err := ShardSet(files[0], files)
	if err != nil {
		return Manifest{}, "", err
	}
	if stem == "" {
		stem = set[0].Path
	}
	var aggregate int64
	for _, f := range set {
		aggregate += f.Size
	}
	manifestDestination, err := ResolveDestination(modelsPath, repository, stem)
	if err != nil {
		return Manifest{}, "", err
	}
	preflight, err := CheckDisk(manifestDestination.Directory, aggregate)
	if err != nil || !preflight.Sufficient {
		if err == nil {
			err = fmt.Errorf("insufficient disk space")
		}
		return Manifest{}, "", err
	}
	manifest := Manifest{SchemaVersion: 2, RepositoryID: repository, Revision: revision, DownloadedAt: time.Now().UTC()}
	for _, file := range set {
		target, err := ResolveDestination(modelsPath, repository, file.Path)
		if err != nil {
			return Manifest{}, "", err
		}
		if err = os.MkdirAll(filepath.Dir(target.Artifact), 0o755); err != nil {
			return Manifest{}, "", err
		}
		cmd := exec.CommandContext(ctx, hfPath, "download", repository, file.Path, "--revision", revision, "--local-dir", target.Directory)
		cmd.Env = os.Environ()
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err = cmd.Run(); err != nil {
			return Manifest{}, "", fmt.Errorf("hf shard download failed: %s", sanitizeDiagnostic(output.String()))
		}
		hash, err := HashFile(ctx, target.Artifact)
		if err != nil {
			return Manifest{}, "", err
		}
		info, err := os.Stat(target.Artifact)
		if err != nil {
			return Manifest{}, "", err
		}
		url, _ := DownloadURL(repository, revision, file.Path)
		manifest.Files = append(manifest.Files, ManifestFile{ModelFile: file.Path, DownloadURL: url, SizeBytes: info.Size(), SHA256: hash, ETag: file.ETag, LFSOID: file.LFSOID})
	}
	manifestPath := manifestDestination.Artifact + ".json"
	if err = WriteManifestAtomic(manifestPath, manifest); err != nil {
		return Manifest{}, "", err
	}
	return manifest, manifestPath, nil
}
