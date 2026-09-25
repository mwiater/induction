package inspect

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Parsed struct {
	Samples int
	Score   float64
	Metrics map[string]float64
}

type LogInfo struct {
	Parsed Parsed
	Path   string
	Status string
}

func Parse(data []byte) (Parsed, error) {
	var v any
	if err := json.Unmarshal(sanitizeNonFiniteJSON(data), &v); err != nil {
		return Parsed{}, fmt.Errorf("malformed result: %w", err)
	}
	return parseValue(v)
}

func ParseLog(dir string) (Parsed, error) {
	info, err := ParseLogInfo(dir)
	if err != nil {
		return Parsed{}, err
	}
	return info.Parsed, nil
}

func ParseLogInfo(dir string) (LogInfo, error) {
	type logFile struct {
		path     string
		modified time.Time
	}
	var files []logFile
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && (filepath.Ext(path) == ".json" || filepath.Ext(path) == ".eval") {
			files = append(files, logFile{path: path, modified: info.ModTime()})
		}
		return nil
	})
	if err != nil {
		return LogInfo{}, err
	}
	sort.Slice(files, func(i, j int) bool { return files[i].modified.After(files[j].modified) })
	for _, f := range files {
		b, e := os.ReadFile(f.path)
		if e != nil {
			continue
		}
		var v any
		if json.Unmarshal(sanitizeNonFiniteJSON(b), &v) == nil {
			if p, e := parseValue(v); e == nil {
				status, _ := v.(map[string]any)["status"].(string)
				return LogInfo{Parsed: p, Path: f.path, Status: status}, nil
			}
		}
	}
	return LogInfo{}, fmt.Errorf("no structured Inspect result found in %s", dir)
}

// sanitizeNonFiniteJSON converts the non-standard NaN and Infinity literals
// emitted by some Inspect scorers into null. Those values can occur in sample
// diagnostics even when the top-level results summary is valid and parseable.
func sanitizeNonFiniteJSON(data []byte) []byte {
	result := make([]byte, 0, len(data))
	inString := false
	escaped := false
	for i := 0; i < len(data); i++ {
		char := data[i]
		if inString {
			result = append(result, char)
			if escaped {
				escaped = false
			} else if char == '\\' {
				escaped = true
			} else if char == '"' {
				inString = false
			}
			continue
		}
		if char == '"' {
			inString = true
			result = append(result, char)
			continue
		}
		if _, size := nonFiniteJSONToken(data[i:]); size > 0 && jsonTokenBoundary(data, i, i+size) {
			result = append(result, "null"...)
			i += size - 1
			continue
		}
		result = append(result, char)
	}
	return result
}

func nonFiniteJSONToken(data []byte) (string, int) {
	for _, token := range []string{"-Infinity", "Infinity", "NaN"} {
		if bytes.HasPrefix(data, []byte(token)) {
			return token, len(token)
		}
	}
	return "", 0
}

func jsonTokenBoundary(data []byte, start, end int) bool {
	validBefore := start == 0 || bytes.Contains([]byte{' ', '\t', '\r', '\n', ':', ',', '[', '{'}, []byte{data[start-1]})
	validAfter := end == len(data) || bytes.Contains([]byte{' ', '\t', '\r', '\n', ',', ']', '}', ':'}, []byte{data[end]})
	return validBefore && validAfter
}
func parseValue(v any) (Parsed, error) {
	var p Parsed
	p.Metrics = map[string]float64{}
	if root, ok := v.(map[string]any); ok {
		if results, ok := root["results"].(map[string]any); ok {
			if n, ok := results["completed_samples"].(float64); ok {
				p.Samples = int(n)
			}
			if p.Samples == 0 {
				if n, ok := results["total_samples"].(float64); ok {
					p.Samples = int(n)
				}
			}
			if scores, ok := results["scores"].([]any); ok {
				headlineMetric := ""
				if headline, ok := results["headline"].(map[string]any); ok {
					headlineMetric, _ = headline["metric"].(string)
				}
				for _, item := range scores {
					score, ok := item.(map[string]any)
					if !ok {
						continue
					}
					metrics, ok := score["metrics"].(map[string]any)
					if !ok {
						continue
					}
					for name, raw := range metrics {
						metric, ok := raw.(map[string]any)
						if !ok {
							continue
						}
						value, ok := metric["value"].(float64)
						if !ok {
							continue
						}
						p.Metrics[name] = value
						if headlineMetric == name || (!foundMetric(p.Metrics) && name != "stderr") {
							p.Score = value
						}
					}
				}
				if len(p.Metrics) > 0 {
					if _, ok := p.Metrics[headlineMetric]; ok {
						p.Score = p.Metrics[headlineMetric]
					}
					return p, nil
				}
			}
		}
	}
	foundScore := false
	var walk func(any)
	walk = func(x any) {
		switch z := x.(type) {
		case map[string]any:
			for k, v := range z {
				switch n := v.(type) {
				case float64:
					if k == "samples" || k == "sample_count" || k == "total_samples" {
						p.Samples = int(n)
					}
					if k == "score" || k == "accuracy" {
						p.Score = n
						foundScore = true
					}
					if k != "score" && k != "samples" && k != "sample_count" && k != "total_samples" {
						if k == "accuracy" || k == "mean" || k == "stderr" || k == "stddev" {
							p.Metrics[k] = n
						}
					}
				case map[string]any:
					if k == "score" || k == "accuracy" {
						if value, ok := n["value"].(float64); ok {
							p.Score = value
							foundScore = true
						}
					}
					walk(n)
				case []any:
					if k == "samples" && p.Samples == 0 {
						p.Samples = len(n)
					}
					walk(n)
				}
			}
		case []any:
			for _, n := range z {
				walk(n)
			}
		}
	}
	walk(v)
	if !foundScore {
		return p, fmt.Errorf("missing score")
	}
	return p, nil
}

func foundMetric(metrics map[string]float64) bool {
	for name := range metrics {
		if name != "stderr" {
			return true
		}
	}
	return false
}
