package pipelineoptimizer

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	induction "github.com/mwiater/induction"
)

type sessionEvidence struct {
	ID        string
	Path      string
	Match     string
	Outcome   string
	UpdatedAt time.Time
	Snapshots int
	Models    []string
	Steps     []string
	Metrics   string
	Error     string
}

func collectSessions(o Options, pipeline *induction.Pipeline, hash, path string) ([]sessionEvidence, []string, error) {
	var paths []string
	explicit := len(o.SessionIDs) > 0 || len(o.SessionFiles) > 0
	for _, id := range o.SessionIDs {
		if !validSessionID(id) {
			return nil, nil, fmt.Errorf("invalid session ID %q", id)
		}
		paths = append(paths, filepath.Join(o.SessionsDir, id+".json"))
	}
	paths = append(paths, o.SessionFiles...)
	if o.AutoSessions {
		entries, err := os.ReadDir(o.SessionsDir)
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("list sessions: %w", err)
		}
		if len(entries) > 2000 {
			return nil, nil, fmt.Errorf("session store has over 2000 entries; select completed sessions explicitly with --session or --session-file")
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.HasSuffix(entry.Name(), ".json") {
				paths = append(paths, filepath.Join(o.SessionsDir, entry.Name()))
			}
		}
	}
	seen := map[string]bool{}
	var selected []sessionEvidence
	var warnings []string
	for _, candidate := range paths {
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			return nil, nil, err
		}
		if seen[absolute] {
			continue
		}
		seen[absolute] = true
		isExplicit := explicit && containsPath(append(append([]string{}, o.SessionFiles...), idPaths(o.SessionsDir, o.SessionIDs)...), candidate)
		e, ok, err := inspectSession(absolute, pipeline, hash, path, isExplicit)
		if err != nil {
			if isExplicit {
				return nil, nil, err
			}
			warnings = append(warnings, fmt.Sprintf("skipped session %s: %v", filepath.Base(candidate), err))
			continue
		}
		if ok {
			stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
			if !isExplicit && altSuffix.MatchString(stem) && e.Match == "same-name-unverified" {
				continue
			}
			selected = append(selected, e)
		} else if isExplicit {
			return nil, nil, fmt.Errorf("session %s is not a completed pipeline session relevant to this selection", filepath.Base(candidate))
		}
	}
	sort.Slice(selected, func(i, j int) bool {
		if rankMatch(selected[i].Match) != rankMatch(selected[j].Match) {
			return rankMatch(selected[i].Match) < rankMatch(selected[j].Match)
		}
		if !selected[i].UpdatedAt.Equal(selected[j].UpdatedAt) {
			return selected[i].UpdatedAt.After(selected[j].UpdatedAt)
		}
		return selected[i].ID < selected[j].ID
	})
	// Diversity: take one of each observed outcome first, then fill by rank.
	var result []sessionEvidence
	used := map[string]bool{}
	outcomes := map[string]bool{}
	for _, e := range selected {
		key := e.Match + ":" + e.Outcome
		if !outcomes[key] {
			result = append(result, e)
			used[e.ID] = true
			outcomes[key] = true
			if len(result) == o.MaxSessions {
				break
			}
		}
	}
	if len(result) < o.MaxSessions {
		for _, e := range selected {
			if !used[e.ID] {
				result = append(result, e)
				if len(result) == o.MaxSessions {
					break
				}
			}
		}
	}
	if len(result) == 0 {
		warnings = append(warnings, "No completed relevant historical sessions were found; analysis uses pipeline and source only.")
	}
	return result, warnings, nil
}

func validSessionID(id string) bool {
	if len(id) != 36 {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !strings.ContainsRune("0123456789abcdefABCDEF", c) {
			return false
		}
	}
	return true
}

func idPaths(dir string, ids []string) []string {
	paths := make([]string, len(ids))
	for i, id := range ids {
		paths[i] = filepath.Join(dir, id+".json")
	}
	return paths
}

func containsPath(paths []string, path string) bool {
	for _, p := range paths {
		if p == path {
			return true
		}
	}
	return false
}

func rankMatch(match string) int {
	switch match {
	case "exact-hash":
		return 0
	case "same-path-unverified":
		return 1
	case "same-name-unverified":
		return 2
	default:
		return 3
	}
}

func inspectSession(path string, pipeline *induction.Pipeline, hash, targetPath string, explicit bool) (sessionEvidence, bool, error) {
	infoBefore, err := os.Lstat(path)
	if err != nil {
		return sessionEvidence{}, false, err
	}
	if !infoBefore.Mode().IsRegular() {
		return sessionEvidence{}, false, fmt.Errorf("session is not a regular file")
	}
	if infoBefore.Size() > 8<<20 {
		return sessionEvidence{}, false, fmt.Errorf("session exceeds 8 MiB read limit")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return sessionEvidence{}, false, err
	}
	infoAfter, err := os.Lstat(path)
	if err != nil {
		return sessionEvidence{}, false, err
	}
	if infoBefore.Size() != infoAfter.Size() || !infoBefore.ModTime().Equal(infoAfter.ModTime()) {
		return sessionEvidence{}, false, fmt.Errorf("session changed while being read")
	}
	var session induction.ChatSession
	if err := json.Unmarshal(data, &session); err != nil {
		return sessionEvidence{}, false, fmt.Errorf("decode session: %w", err)
	}
	if session.Type != "pipeline" {
		return sessionEvidence{}, false, nil
	}
	// Explicit terminal metadata is authoritative. Older binaries lack it;
	// accept only files stable for a minute with strong terminal evidence.
	terminal := session.PipelineStatus == "completed" || session.PipelineStatus == "failed"
	if terminal {
		if session.CompletedAt == nil || session.CompletedAt.IsZero() {
			return sessionEvidence{}, false, nil
		}
	} else if session.PipelineStatus == "" {
		oldEnough := time.Since(infoAfter.ModTime()) >= time.Minute
		terminal = oldEnough && (session.Error != "" || len(session.Snapshots) >= len(pipeline.Steps) && len(pipeline.Steps) > 0 && session.FinalOutput != "")
	}
	if !terminal {
		return sessionEvidence{}, false, nil
	}
	match := "unmatched-explicit"
	if session.Pipeline != nil && session.Pipeline.SHA256 == hash {
		match = "exact-hash"
	} else if session.Pipeline != nil && session.Pipeline.Path == targetPath && session.Pipeline.Path != "" {
		match = "same-path-unverified"
	} else if session.Pipeline != nil && session.Pipeline.Name == pipeline.Name || session.Title == pipeline.Name {
		match = "same-name-unverified"
	}
	if !explicit && match == "unmatched-explicit" {
		return sessionEvidence{}, false, nil
	}
	outcome := "completed"
	if session.Error != "" || session.PipelineStatus == "failed" {
		outcome = "failed"
	}
	models := map[string]bool{}
	var modelList, stepList []string
	steps := map[string]bool{}
	var promptTokens, completionTokens, promptSamples, completionSamples int
	var promptMS, generationMS float64
	var promptTimeSamples, generationTimeSamples int
	for _, snapshot := range session.Snapshots {
		if snapshot != nil && snapshot.ModelID != "" && !models[snapshot.ModelID] {
			models[snapshot.ModelID] = true
			modelList = append(modelList, snapshot.ModelID)
		}
		if snapshot == nil {
			continue
		}
		if snapshot.PipelineStepName != "" && !steps[snapshot.PipelineStepName] {
			steps[snapshot.PipelineStepName] = true
			stepList = append(stepList, snapshot.PipelineStepName)
		}
		for _, interaction := range snapshot.Interaction {
			var response struct {
				Usage struct {
					PromptTokens     *int `json:"prompt_tokens"`
					CompletionTokens *int `json:"completion_tokens"`
				} `json:"usage"`
				Timings struct {
					PromptMS    *float64 `json:"prompt_ms"`
					PredictedMS *float64 `json:"predicted_ms"`
				} `json:"timings"`
			}
			if json.Unmarshal([]byte(interaction.Response), &response) == nil {
				if response.Usage.PromptTokens != nil {
					promptTokens += *response.Usage.PromptTokens
					promptSamples++
				}
				if response.Usage.CompletionTokens != nil {
					completionTokens += *response.Usage.CompletionTokens
					completionSamples++
				}
				if response.Timings.PromptMS != nil && *response.Timings.PromptMS >= 0 {
					promptMS += *response.Timings.PromptMS
					promptTimeSamples++
				}
				if response.Timings.PredictedMS != nil && *response.Timings.PredictedMS >= 0 {
					generationMS += *response.Timings.PredictedMS
					generationTimeSamples++
				}
			}
		}
	}
	sort.Strings(modelList)
	metrics := fmt.Sprintf("snapshot_count=%d", len(session.Snapshots))
	if promptSamples > 0 {
		metrics += fmt.Sprintf(", prompt_tokens=%d (n=%d)", promptTokens, promptSamples)
	}
	if completionSamples > 0 {
		metrics += fmt.Sprintf(", completion_tokens=%d (n=%d)", completionTokens, completionSamples)
	}
	if promptTimeSamples > 0 {
		metrics += fmt.Sprintf(", prompt_ms=%.0f (n=%d)", promptMS, promptTimeSamples)
	}
	if generationTimeSamples > 0 {
		metrics += fmt.Sprintf(", generation_ms=%.0f (n=%d)", generationMS, generationTimeSamples)
	}
	return sessionEvidence{ID: session.ID, Path: path, Match: match, Outcome: outcome, UpdatedAt: session.UpdatedAt, Snapshots: len(session.Snapshots), Models: modelList, Steps: stepList, Metrics: metrics, Error: truncate(session.Error, 300)}, true, nil
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + " [truncated]"
}
