// Package induction contains the core implementation for the public Induction API.
package induction

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// WriteDashboardMetrics atomically writes an indented dashboard artifact.
func WriteDashboardMetrics(path string, metrics *DashboardMetrics) error {
	if metrics == nil {
		return errors.New("dashboard metrics are nil")
	}
	contents, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dashboard metrics: %w", err)
	}
	contents = append(contents, '\n')
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create dashboard directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".session_metrics-*.tmp")
	if err != nil {
		return fmt.Errorf("create dashboard temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set dashboard permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write dashboard metrics: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync dashboard metrics: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close dashboard metrics: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace dashboard metrics: %w", err)
	}
	return nil
}

// dashboardDataPlaceholder identifies a fixed value used internally by this package.
const dashboardDataPlaceholder = "const DASHBOARD_DATA = // insert data/dashboard/session_metrics.json;"

const dashboardChartLibraryPlaceholder = "/* insert internal/templates/chart.umd.min.js */"

// WriteDashboardHTML embeds the dashboard metrics in the HTML template and
// atomically writes the resulting self-contained dashboard artifact.
func WriteDashboardHTML(templatePath, path string, metrics *DashboardMetrics) error {
	if metrics == nil {
		return errors.New("dashboard metrics are nil")
	}
	contents, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("read dashboard template: %w", err)
	}
	if bytes.Count(contents, []byte(dashboardDataPlaceholder)) != 1 {
		return fmt.Errorf("dashboard template must contain exactly one %q placeholder", dashboardDataPlaceholder)
	}
	data, err := json.MarshalIndent(metrics, "", "  ")
	if err != nil {
		return fmt.Errorf("encode dashboard data: %w", err)
	}
	replacement := append([]byte("const DASHBOARD_DATA = "), data...)
	replacement = append(replacement, ';')
	contents = bytes.Replace(contents, []byte(dashboardDataPlaceholder), replacement, 1)
	if bytes.Contains(contents, []byte(dashboardChartLibraryPlaceholder)) {
		libraryPath := filepath.Join(filepath.Dir(templatePath), "chart.umd.min.js")
		library, err := os.ReadFile(libraryPath)
		if err != nil {
			return fmt.Errorf("read dashboard chart library: %w", err)
		}
		contents = bytes.Replace(contents, []byte(dashboardChartLibraryPlaceholder), library, 1)
	}

	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create dashboard directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".dashboard-*.tmp")
	if err != nil {
		return fmt.Errorf("create dashboard temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set dashboard permissions: %w", err)
	}
	if _, err := temporary.Write(contents); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write dashboard HTML: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync dashboard HTML: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close dashboard HTML: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace dashboard HTML: %w", err)
	}
	return nil
}

// GenerateDashboardMetrics builds and writes the default dashboard artifacts.
func GenerateDashboardMetrics(options DashboardGenerateOptions) (*DashboardMetrics, error) {
	evalResultsDirectory := options.EvalResultsDirectory
	if evalResultsDirectory == "" {
		evalResultsDirectory = DefaultDashboardEvalResultsDirectory
	}
	ignoreFilePath := options.IgnoreFilePath
	if ignoreFilePath == "" {
		ignoreFilePath = DefaultDashboardIgnorePath
	}
	metrics, err := buildDashboardMetricsFromDirectoriesWithIgnore(options.SessionsDirectory, evalResultsDirectory, ignoreFilePath)
	if err != nil {
		return nil, err
	}
	ignoredModels, err := loadDashboardIgnoredModels(ignoreFilePath)
	if err != nil {
		return nil, err
	}
	metrics.SIA, err = loadDashboardSIAMetricsWithIgnore(DefaultDashboardSIAMetricsPath, ignoredModels)
	if err != nil {
		return nil, err
	}
	if err := WriteDashboardMetrics(DefaultDashboardMetricsPath, metrics); err != nil {
		return nil, err
	}
	if err := WriteDashboardHTML(DefaultDashboardTemplatePath, DefaultDashboardHTMLPath, metrics); err != nil {
		return nil, err
	}
	return metrics, nil
}

// loadDashboardSIAMetrics performs the internal operation described by its signature.
// Additional context: Keep the compact report needed for dashboard inspection. The raw report's question manifest, prompt prefix, and per-rotation results can account for tens of megabytes but are not used by the dashboard.
func loadDashboardSIAMetrics(path string) (json.RawMessage, error) {
	return loadDashboardSIAMetricsWithIgnore(path, nil)
}

func loadDashboardSIAMetricsWithIgnore(path string, ignoredModels map[string]bool) (json.RawMessage, error) {
	contents, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return json.RawMessage("[]"), nil
	}
	if err != nil {
		return nil, fmt.Errorf("read SIA metrics: %w", err)
	}
	var value map[string]any
	if err := json.Unmarshal(contents, &value); err != nil {
		return nil, fmt.Errorf("decode SIA metrics %s: %w", path, err)
	}
	if value == nil {
		return nil, fmt.Errorf("decode SIA metrics %s: expected a JSON object", path)
	}

	compact := make(map[string]any)
	for _, key := range []string{
		"schema_version", "run_id", "timestamp", "items", "rotations_per_item",
		"permutation_sets", "model_order", "settings", "notes",
	} {
		if field, ok := value[key]; ok {
			compact[key] = field
		}
	}
	if models, ok := value["models"].(map[string]any); ok {
		compactModels := make(map[string]any, len(models))
		for modelName, rawModel := range models {
			if ignoredModels[modelName] {
				continue
			}
			model, ok := rawModel.(map[string]any)
			if !ok {
				continue
			}
			compactModel := make(map[string]any)
			for _, key := range []string{
				"status", "requested_model", "verified_model", "model_identity_status",
				"started_at", "finished_at", "complete", "incomplete", "output_fingerprint", "metrics",
			} {
				if field, ok := model[key]; ok {
					compactModel[key] = field
				}
			}
			compactModels[modelName] = compactModel
		}
		compact["models"] = compactModels
	}
	if modelOrder, ok := value["model_order"].([]any); ok {
		filteredOrder := make([]any, 0, len(modelOrder))
		for _, rawModel := range modelOrder {
			modelName, ok := rawModel.(string)
			if !ok || !ignoredModels[modelName] {
				filteredOrder = append(filteredOrder, rawModel)
			}
		}
		compact["model_order"] = filteredOrder
	}
	pruned, err := json.Marshal(compact)
	if err != nil {
		return nil, fmt.Errorf("prune SIA metrics %s: %w", path, err)
	}
	return json.RawMessage(pruned), nil
}
