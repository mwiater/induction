// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// BuildDashboardMetrics builds the rebuildable dashboard projection without contacting a server.
func BuildDashboardMetrics(sessionsDirectory string) (*DashboardMetrics, error) {
	if sessionsDirectory == "" {
		sessionsDirectory = DefaultDashboardSessionsDirectory
	}
	return buildDashboardMetricsFromDirectoriesWithIgnore(sessionsDirectory, DefaultDashboardEvalResultsDirectory, DefaultDashboardIgnorePath)
}

// buildDashboardMetricsFromDirectories performs the internal operation described by its signature.
func buildDashboardMetricsFromDirectories(sessionsDirectory, evalResultsDirectory string) (*DashboardMetrics, error) {
	return buildDashboardMetricsFromDirectoriesWithIgnore(sessionsDirectory, evalResultsDirectory, DefaultDashboardIgnorePath)
}

// buildDashboardMetricsFromDirectoriesWithIgnore performs the internal operation described by its signature.
func buildDashboardMetricsFromDirectoriesWithIgnore(sessionsDirectory, evalResultsDirectory, ignoreFilePath string) (*DashboardMetrics, error) {
	if sessionsDirectory == "" {
		sessionsDirectory = DefaultDashboardSessionsDirectory
	}
	ignoredModels, err := loadDashboardIgnoredModels(ignoreFilePath)
	if err != nil {
		return nil, err
	}
	sources, err := scanDashboardSessions(sessionsDirectory)
	if err != nil {
		return nil, err
	}
	source := DashboardSource{Directory: sessionsDirectory, SessionFiles: len(sources)}
	for _, item := range sources {
		source.SessionsLoaded++
		source.SnapshotsSeen += len(item.session.Snapshots)
	}
	result, err := buildDashboardMetrics(sources, source, ignoredModels)
	if err != nil {
		return nil, err
	}
	evalSources, err := scanDashboardEvalResults(evalResultsDirectory)
	if err != nil {
		return nil, err
	}
	result.Source.EvalFilesSeen = len(evalSources)
	for _, evalSource := range evalSources {
		if evalSource.result == nil || ignoredModels[evalSource.result.Model.Name] || !mergeDashboardEval(result, evalSource.result) {
			result.Source.EvalsSkipped++
			continue
		}
		result.Source.EvalsIncluded++
	}
	separateDashboardSystemOneModels(result, loadDashboardSystemOneModelIDs(DefaultDashboardModelCatalogPath))
	result.Source.Models = len(result.Models)
	result.GeneratedAt = time.Now().UTC()
	return result, nil
}

// loadDashboardSystemOneModelIDs reads the local /v1/models snapshot without
// making dashboard generation depend on a live server.
func loadDashboardSystemOneModelIDs(path string) map[string]bool {
	ids := make(map[string]bool)
	contents, err := os.ReadFile(path)
	if err != nil {
		return ids
	}
	var catalog struct {
		Data []struct {
			ID           string   `json:"id"`
			OutputModes  []string `json:"output_modalities"`
			Architecture struct {
				OutputModes []string `json:"output_modalities"`
			} `json:"architecture"`
		} `json:"data"`
	}
	if json.Unmarshal(contents, &catalog) != nil {
		return ids
	}
	for _, model := range catalog.Data {
		for _, mode := range append(model.OutputModes, model.Architecture.OutputModes...) {
			if strings.EqualFold(strings.TrimSpace(mode), "decisions") && strings.TrimSpace(model.ID) != "" {
				ids[model.ID] = true
				break
			}
		}
	}
	return ids
}

// separateDashboardSystemOneModels keeps decision-only model telemetry in its
// own dashboard area so ordinary performance and behavior views remain text
// generation data only.
func separateDashboardSystemOneModels(metrics *DashboardMetrics, ids map[string]bool) {
	if metrics == nil || len(ids) == 0 {
		return
	}
	ordinary := make([]DashboardModelData, 0, len(metrics.Models))
	systemOne := make([]DashboardModelData, 0)
	known := make(map[string]bool)
	for _, model := range metrics.Models {
		if ids[model.ModelID] {
			systemOne = append(systemOne, model)
			known[model.ModelID] = true
		} else {
			ordinary = append(ordinary, model)
		}
	}
	for id := range ids {
		if !known[id] {
			systemOne = append(systemOne, DashboardModelData{ModelID: id, Observations: []DashboardSnapshotObservation{}})
		}
	}
	sort.Slice(systemOne, func(i, j int) bool { return systemOne[i].ModelID < systemOne[j].ModelID })
	metrics.Models = ordinary
	metrics.SystemOneModels = systemOne
	metrics.ContextPressure = filterDashboardModelMap(metrics.ContextPressure, ids)
	metrics.LatencyPressure = filterDashboardModelMap(metrics.LatencyPressure, ids)
	metrics.CachePressure = filterDashboardModelMap(metrics.CachePressure, ids)
	metrics.ReasoningPressure = filterDashboardModelMap(metrics.ReasoningPressure, ids)
	metrics.WorkloadPressure = filterDashboardModelMap(metrics.WorkloadPressure, ids)
	metrics.Reliability = filterDashboardModelMap(metrics.Reliability, ids)
	metrics.RuntimeConfigs = filterDashboardConfigs(metrics.RuntimeConfigs, ids)
	metrics.GenerationConfigs = filterDashboardGenerationConfigs(metrics.GenerationConfigs, ids)
	metrics.LoadEvents = filterDashboardLoadEvents(metrics.LoadEvents, ids)
}

func filterDashboardModelMap[T any](values map[string]T, excluded map[string]bool) map[string]T {
	for id := range excluded {
		delete(values, id)
	}
	return values
}

func filterDashboardConfigs(values []DashboardRuntimeConfig, excluded map[string]bool) []DashboardRuntimeConfig {
	result := values[:0]
	for _, value := range values {
		if !excluded[value.ModelID] {
			result = append(result, value)
		}
	}
	return result
}

func filterDashboardGenerationConfigs(values []DashboardGenerationConfig, excluded map[string]bool) []DashboardGenerationConfig {
	result := values[:0]
	for _, value := range values {
		if !excluded[value.ModelID] {
			result = append(result, value)
		}
	}
	return result
}

func filterDashboardLoadEvents(values []DashboardLoadEvent, excluded map[string]bool) []DashboardLoadEvent {
	result := values[:0]
	for _, value := range values {
		if !excluded[value.ModelID] {
			result = append(result, value)
		}
	}
	return result
}

// loadDashboardIgnoredModels performs the internal operation described by its signature.
func loadDashboardIgnoredModels(path string) (map[string]bool, error) {
	ignored := make(map[string]bool)
	if strings.TrimSpace(path) == "" {
		return ignored, nil
	}
	file, err := os.Open(path)
	if os.IsNotExist(err) {
		return ignored, nil
	}
	if err != nil {
		return nil, fmt.Errorf("generate dashboard: open ignore file %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		modelID := strings.TrimSpace(scanner.Text())
		if modelID == "" || strings.HasPrefix(modelID, "#") {
			continue
		}
		ignored[modelID] = true
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("generate dashboard: read ignore file %q: %w", path, err)
	}
	return ignored, nil
}

// scanDashboardSessions performs the internal operation described by its signature.
func scanDashboardSessions(directory string) ([]dashboardSessionSource, error) {
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []dashboardSessionSource{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("generate dashboard: list sessions directory %q: %w", directory, err)
	}
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			paths = append(paths, filepath.Join(directory, entry.Name()))
		}
	}
	sort.Strings(paths)
	result := make([]dashboardSessionSource, 0, len(paths))
	seen := make(map[string]string)
	for _, path := range paths {
		session, loadErr := loadChatSessionFromPath(path)
		if loadErr != nil {
			return nil, fmt.Errorf("generate dashboard: load session %q: %w", path, loadErr)
		}
		if previous, exists := seen[session.ID]; exists {
			return nil, fmt.Errorf("generate dashboard: duplicate session ID %q in %q and %q", session.ID, previous, path)
		}
		seen[session.ID] = path
		result = append(result, dashboardSessionSource{path: path, session: session})
	}
	return result, nil
}

// scanDashboardEvalResults performs the internal operation described by its signature.
func scanDashboardEvalResults(directory string) ([]dashboardEvalSource, error) {
	if directory == "" {
		directory = DefaultDashboardEvalResultsDirectory
	}
	if _, err := os.Stat(directory); os.IsNotExist(err) {
		return []dashboardEvalSource{}, nil
	} else if err != nil {
		return nil, fmt.Errorf("generate dashboard: stat eval results directory %q: %w", directory, err)
	}
	var paths []string
	if err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			paths = append(paths, path)
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("generate dashboard: scan eval results directory %q: %w", directory, err)
	}
	sort.Strings(paths)
	tags := loadDashboardEvalTags(DefaultDashboardEvalConfigPath)
	result := make([]dashboardEvalSource, 0, len(paths))
	for _, path := range paths {
		loaded, loadErr := loadDashboardEvalResult(path, tags)
		if loadErr != nil {
			result = append(result, dashboardEvalSource{path: path})
			continue
		}
		result = append(result, dashboardEvalSource{path: path, result: loaded})
	}
	return result, nil
}

// loadDashboardEvalTags performs the internal operation described by its signature.
func loadDashboardEvalTags(path string) map[string]string {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var config dashboardEvalTagConfig
	if yaml.Unmarshal(contents, &config) != nil {
		return nil
	}
	tags := make(map[string]string, len(config.Evals))
	for _, definition := range config.Evals {
		if strings.TrimSpace(definition.Name) != "" && strings.TrimSpace(definition.Tag) != "" {
			tags[definition.Name] = definition.Tag
		}
	}
	return tags
}

// loadDashboardEvalResult performs the internal operation described by its signature.
func loadDashboardEvalResult(path string, tags map[string]string) (*dashboardEvalResult, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var result dashboardEvalResult
	if err := json.Unmarshal(contents, &result); err != nil {
		return nil, err
	}
	for index := range result.Benchmarks {
		if strings.TrimSpace(result.Benchmarks[index].Tag) == "" {
			result.Benchmarks[index].Tag = tags[result.Benchmarks[index].Name]
		}
	}
	if !validDashboardEvalResult(&result) {
		return nil, errors.New("invalid or incomplete eval result")
	}
	return &result, nil
}

// validDashboardEvalResult performs the internal operation described by its signature.
func validDashboardEvalResult(result *dashboardEvalResult) bool {
	if result == nil || strings.TrimSpace(result.Model.Name) == "" || strings.TrimSpace(result.Suite.Name) == "" || (result.Status != "success" && result.Status != "running" && result.Status != "failed") {
		return false
	}
	for _, benchmark := range result.Benchmarks {
		if validDashboardBenchmark(benchmark) {
			return true
		}
	}
	return false
}

// validDashboardBenchmark performs the internal operation described by its signature.
func validDashboardBenchmark(benchmark dashboardEvalBenchmarkWire) bool {
	if strings.TrimSpace(benchmark.Name) == "" || strings.TrimSpace(benchmark.Task) == "" || benchmark.Samples <= 0 || (benchmark.Limit > 0 && benchmark.Samples < benchmark.Limit) || !finiteDashboardNumber(benchmark.Score) {
		return false
	}
	for _, value := range benchmark.Metrics {
		if !finiteDashboardNumber(value) {
			return false
		}
	}
	return true
}

// finiteDashboardNumber performs the internal operation described by its signature.
func finiteDashboardNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

// mergeDashboardEval performs the internal operation described by its signature.
func mergeDashboardEval(metrics *DashboardMetrics, result *dashboardEvalResult) bool {
	if metrics == nil || !validDashboardEvalResult(result) {
		return false
	}
	modelIndex := -1
	for index := range metrics.Models {
		if metrics.Models[index].ModelID == result.Model.Name {
			modelIndex = index
			break
		}
	}
	if modelIndex < 0 {
		metrics.Models = append(metrics.Models, DashboardModelData{ModelID: result.Model.Name, Observations: []DashboardSnapshotObservation{}})
		modelIndex = len(metrics.Models) - 1
	}
	converted := DashboardEvalData{RunID: result.RunID, Suite: DashboardEvalSuite{Name: result.Suite.Name, ConfigHash: result.Suite.ConfigHash}, Engine: result.Engine, Server: result.Server, CompletedAt: result.CompletedAt, DurationMS: result.DurationMS, Status: result.Status}
	if result.Aggregate != nil && strings.TrimSpace(result.Aggregate.Method) != "" && finiteDashboardNumber(result.Aggregate.Score) {
		aggregate := DashboardEvalAggregate{Score: result.Aggregate.Score, Method: result.Aggregate.Method}
		converted.Aggregate = &aggregate
	}
	converted.Benchmarks = make([]DashboardEvalBenchmark, 0, len(result.Benchmarks))
	for _, benchmark := range result.Benchmarks {
		if !validDashboardBenchmark(benchmark) {
			continue
		}
		metricsCopy := make(map[string]float64, len(benchmark.Metrics))
		for key, value := range benchmark.Metrics {
			metricsCopy[key] = value
		}
		converted.Benchmarks = append(converted.Benchmarks, DashboardEvalBenchmark{Name: benchmark.Name, Tag: benchmark.Tag, Task: benchmark.Task, Limit: benchmark.Limit, MaxTokens: benchmark.MaxTokens, Samples: benchmark.Samples, Score: benchmark.Score, Metrics: metricsCopy})
	}
	metrics.Models[modelIndex].Evals = append(metrics.Models[modelIndex].Evals, converted)
	sort.SliceStable(metrics.Models[modelIndex].Evals, func(i, j int) bool {
		left, right := metrics.Models[modelIndex].Evals[i], metrics.Models[modelIndex].Evals[j]
		if left.Suite.Name != right.Suite.Name {
			return left.Suite.Name < right.Suite.Name
		}
		if !left.CompletedAt.Equal(right.CompletedAt) {
			return left.CompletedAt.Before(right.CompletedAt)
		}
		return left.RunID < right.RunID
	})
	sort.Slice(metrics.Models, func(i, j int) bool { return metrics.Models[i].ModelID < metrics.Models[j].ModelID })
	return true
}
