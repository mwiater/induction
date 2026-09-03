package modelmanager

import (
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DetectQuantization recognizes common GGUF quantization tokens without
// excluding unfamiliar artifact names.
func DetectQuantization(name string) string {
	upper := strings.ToUpper(filepath.Base(name))
	match := regexp.MustCompile(`(?:^|[-_.])(IQ[0-9](?:_[A-Z0-9]+)*|Q[0-9](?:_[A-Z0-9]+)*|BF16|F16|F32)(?:[-.]|$)`).FindStringSubmatch(upper)
	if len(match) > 1 {
		return match[1]
	}
	return ""
}

func matchAny(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
		if ok, _ := filepath.Match(p, filepath.Base(name)); ok {
			return true
		}
	}
	return false
}

// FilterFiles applies configured globs, defaults to GGUF when available, and
// stably promotes preferred quantizations.
func FilterFiles(files []ModelFile, include, exclude, preferred []string, revealAll bool) []ModelFile {
	filtered := make([]ModelFile, 0, len(files))
	hasGGUF := false
	for _, f := range files {
		if strings.EqualFold(filepath.Ext(f.Path), ".gguf") {
			hasGGUF = true
		}
	}
	for _, f := range files {
		if len(include) > 0 && !matchAny(include, f.Path) {
			continue
		}
		if matchAny(exclude, f.Path) {
			continue
		}
		if !revealAll && hasGGUF && !strings.EqualFold(filepath.Ext(f.Path), ".gguf") {
			continue
		}
		filtered = append(filtered, f)
	}
	order := map[string]int{}
	for i, q := range preferred {
		order[strings.ToUpper(q)] = i
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		ai, aok := order[strings.ToUpper(filtered[i].Quantization)]
		aj, bok := order[strings.ToUpper(filtered[j].Quantization)]
		if aok != bok {
			return aok
		}
		return aok && ai < aj
	})
	return filtered
}

// DownloadURL returns a revision-pinned, safely escaped Hugging Face URL.
func DownloadURL(repository, revision, filename string) (string, error) {
	if revision == "" {
		return "", fmt.Errorf("immutable revision is required")
	}
	parts := strings.Split(filename, "/")
	for i, p := range parts {
		if p == "" || p == "." || p == ".." {
			return "", fmt.Errorf("unsafe filename %q", filename)
		}
		parts[i] = url.PathEscape(p)
	}
	return "https://huggingface.co/" + strings.Trim(repository, "/") + "/resolve/" + url.PathEscape(revision) + "/" + strings.Join(parts, "/"), nil
}
