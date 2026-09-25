package eval

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func SafeModelName(model string) string {
	model = strings.TrimSpace(model)
	var b strings.Builder
	for _, r := range model {
		if r == '/' || r == '\\' || r == ':' {
			b.WriteString("__")
		} else {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown-model"
	}
	return b.String()
}

func SaveResult(root string, result *Result) (string, error) {
	if result == nil {
		return "", fmt.Errorf("result is nil")
	}
	dir := filepath.Join(root, "data", "evals", "results", SafeModelName(result.Model.Name))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, result.Suite.Name+".json")
	tmp, err := os.CreateTemp(dir, ".result-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	enc := json.NewEncoder(tmp)
	enc.SetIndent("", "  ")
	if err = enc.Encode(result); err == nil {
		err = tmp.Sync()
	}
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}
	if err = os.Rename(tmpName, path); err != nil {
		return "", err
	}
	return path, nil
}

func ResultPath(root, model, suite string) string {
	return filepath.Join(root, "data", "evals", "results", SafeModelName(model), suite+".json")
}

func LoadResult(root, model, suite string) (*Result, error) {
	path := ResultPath(root, model, suite)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result Result
	if err := json.Unmarshal(b, &result); err != nil {
		return nil, fmt.Errorf("decode eval result %q: %w", path, err)
	}
	return &result, nil
}
