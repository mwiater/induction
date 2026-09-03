package modelmanager

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// Installation pairs a validated manifest with the artifact paths it describes.
type Installation struct {
	Manifest      Manifest `json:"manifest"`
	ArtifactPath  string   `json:"artifactPath"`
	ArtifactPaths []string `json:"artifactPaths,omitempty"`
	ManifestPath  string   `json:"manifestPath"`
}

// InstalledIndex contains valid installations and warnings from a model scan.
type InstalledIndex struct {
	Installations []Installation `json:"installations"`
	Warnings      []string       `json:"warnings,omitempty"`
}

// BuildInstalledIndex scans modelsPath and returns only installations with
// safe paths and present artifacts.
func BuildInstalledIndex(modelsPath string) (InstalledIndex, error) {
	base, err := filepath.Abs(modelsPath)
	if err != nil {
		return InstalledIndex{}, err
	}
	index := InstalledIndex{}
	seen := map[string]struct{}{}
	err = filepath.WalkDir(base, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			index.Warnings = append(index.Warnings, walkErr.Error())
			return nil
		}
		if entry.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			index.Warnings = append(index.Warnings, err.Error())
			return nil
		}
		var manifest Manifest
		if json.Unmarshal(data, &manifest) != nil || (manifest.SchemaVersion != 1 && manifest.SchemaVersion != 2) || manifest.RepositoryID == "" || manifest.Revision == "" {
			return nil
		}
		if manifest.SchemaVersion == 2 {
			if len(manifest.Files) == 0 {
				return nil
			}
			valid := true
			paths := make([]string, 0, len(manifest.Files))
			for _, file := range manifest.Files {
				destination, resolveErr := ResolveDestination(base, manifest.RepositoryID, file.ModelFile)
				if resolveErr != nil {
					valid = false
					break
				}
				if _, statErr := os.Stat(destination.Artifact); statErr != nil {
					index.Warnings = append(index.Warnings, "manifest shard missing: "+destination.Artifact)
					valid = false
					break
				}
				paths = append(paths, destination.Artifact)
			}
			if valid {
				index.Installations = append(index.Installations, Installation{Manifest: manifest, ArtifactPaths: paths, ManifestPath: path})
			}
			return nil
		}
		if manifest.ModelFile == "" {
			return nil
		}
		destination, err := ResolveDestination(base, manifest.RepositoryID, manifest.ModelFile)
		if err != nil || destination.Manifest != path {
			index.Warnings = append(index.Warnings, fmt.Sprintf("ignored unsafe manifest %s", path))
			return nil
		}
		if _, err = os.Stat(destination.Artifact); err != nil {
			index.Warnings = append(index.Warnings, fmt.Sprintf("manifest artifact missing: %s", destination.Artifact))
			return nil
		}
		key := manifest.RepositoryID + "\x00" + manifest.ModelFile
		if _, ok := seen[key]; ok {
			index.Warnings = append(index.Warnings, "duplicate installation: "+manifest.RepositoryID+"/"+manifest.ModelFile)
			return nil
		}
		seen[key] = struct{}{}
		index.Installations = append(index.Installations, Installation{Manifest: manifest, ArtifactPath: destination.Artifact, ArtifactPaths: []string{destination.Artifact}, ManifestPath: path})
		return nil
	})
	if os.IsNotExist(err) {
		return index, nil
	}
	return index, err
}

// RepositoryCount returns the number of installations belonging to repository.
func (i InstalledIndex) RepositoryCount(repository string) int {
	count := 0
	for _, item := range i.Installations {
		if item.Manifest.RepositoryID == repository {
			count++
		}
	}
	return count
}

// FileInstallState describes whether a requested model file is installed.
type FileInstallState string

const (
	// FileNotInstalled indicates that the requested artifact is absent.
	FileNotInstalled FileInstallState = "NOT INSTALLED"
	// FileInstalled indicates that the artifact and revision match.
	FileInstalled FileInstallState = "INSTALLED"
	// FileDifferentRevision indicates that the artifact exists at another revision.
	FileDifferentRevision FileInstallState = "INSTALLED (different revision)"
	// FileUntracked indicates that a file exists without a matching manifest.
	FileUntracked FileInstallState = "FILE EXISTS (untracked)"
)

// FileState compares a requested file with the indexed installation state.
func (i InstalledIndex) FileState(modelsPath, repository, file, revision string) FileInstallState {
	for _, item := range i.Installations {
		if item.Manifest.RepositoryID == repository && item.Manifest.ModelFile == file {
			if item.Manifest.Revision == revision {
				return FileInstalled
			}
			return FileDifferentRevision
		}
	}
	destination, err := ResolveDestination(modelsPath, repository, file)
	if err == nil {
		if _, err = os.Stat(destination.Artifact); err == nil {
			return FileUntracked
		}
	}
	return FileNotInstalled
}
