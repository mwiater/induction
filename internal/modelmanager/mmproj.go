package modelmanager

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MMProjFiles returns the vision projector artifacts published by a Hub
// repository. The names used by llama.cpp conventionally contain mmproj and
// use the GGUF format.
func MMProjFiles(files []ModelFile) []ModelFile {
	result := make([]ModelFile, 0)
	for _, file := range files {
		name := strings.ToLower(filepath.Base(file.Path))
		if strings.Contains(name, "mmproj") && strings.EqualFold(filepath.Ext(name), ".gguf") {
			result = append(result, file)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

// MMProjInstalled reports whether the exact projector artifact exists locally.
// An untracked file still counts as downloaded; the command is a readiness
// check and should not require a second download merely to create a manifest.
func MMProjInstalled(modelsPath, repository, file string) bool {
	destination, err := ResolveDestination(modelsPath, repository, file)
	if err != nil {
		return false
	}
	_, err = os.Stat(destination.Artifact)
	return err == nil
}
