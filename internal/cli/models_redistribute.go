package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	induction "github.com/mwiater/induction"
	"github.com/spf13/cobra"
)

// redistributeReport is the read-only inventory used by later redistribution
// stages. Keep the wire shape explicit so it remains useful to scripts.
type redistributeReport struct {
	Models    []redistributeModel    `json:"models"`
	Pipelines []redistributePipeline `json:"pipelines"`
}

// redistributeModel describes one server model and the pipeline usage count
// observed for that model.
type redistributeModel struct {
	ModelID           string   `json:"model_id"`
	InputModalities   []string `json:"input_modalities"`
	OutputModalities  []string `json:"output_modalities"`
	CurrentUsageCount int      `json:"current_usage_count"`
}

// redistributePipeline describes one discovered pipeline and its step
// modality requirements.
type redistributePipeline struct {
	PipelineFilename string             `json:"pipeline_filename"`
	Steps            []redistributeStep `json:"steps"`
}

// redistributeStep describes the input, output, and configured model for one
// pipeline step.
type redistributeStep struct {
	StepName             string   `json:"step_name"`
	StepNumber           int      `json:"step_number"`
	StepInputModalities  []string `json:"step_input_modalities"`
	StepOutputModalities []string `json:"step_output_modalities"`
	ConfiguredModelID    string   `json:"configured_model_id,omitempty"`
}

// newModelsRedistributeCommand creates the read-only model and pipeline
// modality inventory command.
func newModelsRedistributeCommand(configPath *string) *cobra.Command {
	var pipelinesDir string
	cmd := &cobra.Command{
		Use:   "redistribute",
		Short: "inspect model and pipeline modality requirements",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := induction.LoadConfig(*configPath)
			if err != nil {
				return err
			}
			report, err := buildRedistributeReport(cmd.Context(), cfg.Server, pipelinesDir, nil)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(report)
		},
	}
	cmd.Flags().StringVar(&pipelinesDir, "pipelines-dir", "pipelines", "directory containing pipeline YAML files")
	return cmd
}

// buildRedistributeReport fetches the server model catalog and combines it
// with the configured model usage and modality requirements in pipeline YAML
// files under pipelinesDir.
func buildRedistributeReport(ctx context.Context, endpoint, pipelinesDir string, httpClient *http.Client) (*redistributeReport, error) {
	options := make([]induction.ClientOption, 0, 1)
	if httpClient != nil {
		options = append(options, induction.WithHTTPClient(httpClient))
	}
	catalog, err := induction.NewClient(ctx, endpoint, options...).ListModelCatalog(ctx)
	if err != nil {
		return nil, fmt.Errorf("fetch model catalog: %w", err)
	}

	paths, err := filepath.Glob(filepath.Join(pipelinesDir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("find pipelines: %w", err)
	}
	sort.Strings(paths)
	report := &redistributeReport{Models: make([]redistributeModel, 0, len(catalog)), Pipelines: make([]redistributePipeline, 0, len(paths))}
	usage := make(map[string]int)
	for _, path := range paths {
		pipeline, loadErr := induction.LoadPipeline(path)
		if loadErr != nil {
			return nil, loadErr
		}
		item := redistributePipeline{PipelineFilename: path, Steps: make([]redistributeStep, 0, len(pipeline.Steps))}
		for i, step := range pipeline.Steps {
			if strings.TrimSpace(step.Model) != "" {
				usage[step.Model]++
			}
			item.Steps = append(item.Steps, redistributeStep{
				StepName: step.Name, StepNumber: i + 1,
				StepInputModalities:  pipelineStepInputModalities(step),
				StepOutputModalities: pipelineStepOutputModalities(step),
				ConfiguredModelID:    step.Model,
			})
		}
		report.Pipelines = append(report.Pipelines, item)
	}
	for _, model := range catalog {
		report.Models = append(report.Models, redistributeModel{
			ModelID:           model.ID,
			InputModalities:   nonEmptyModalities(model.InputModes, []string{"text"}),
			OutputModalities:  modelOutputModalities(model.OutputModes),
			CurrentUsageCount: usage[model.ID],
		})
	}
	return report, nil
}

// pipelineStepInputModalities infers the input modalities required by a
// pipeline step from its configured attachment fields.
func pipelineStepInputModalities(step induction.PipelineStep) []string {
	if step.Image != "" {
		return []string{"text", "image"}
	}
	if step.Document != "" {
		return []string{"text"}
	}
	return []string{"text"}
}

// pipelineStepOutputModalities infers the output modality produced by a
// pipeline step from its decision, JSON, or response-format configuration.
func pipelineStepOutputModalities(step induction.PipelineStep) []string {
	if inductionStepIsDecision(step) {
		return []string{"decisions"}
	}
	if step.Output != nil && strings.EqualFold(step.Output.Type, "json") {
		return []string{"json"}
	}
	if step.ResponseFormat != nil {
		if strings.EqualFold(step.ResponseFormat.Type, "json_object") || strings.EqualFold(step.ResponseFormat.Type, "json_schema") {
			return []string{"json_object"}
		}
	}
	if step.JSONSchema != nil {
		return []string{"json_object"}
	}
	return []string{"text"}
}

// inductionStepIsDecision reports whether step uses decision or classification
// routing.
func inductionStepIsDecision(step induction.PipelineStep) bool {
	return step.Decision != nil || step.Classification != nil
}

// modelOutputModalities preserves advertised model output modalities and adds
// the JSON forms supported by the redistribution inventory when appropriate.
func modelOutputModalities(actual []string) []string {
	if len(actual) == 0 {
		return []string{"text", "json", "json_object"}
	}
	result := append([]string(nil), actual...)
	for _, mode := range []string{"json", "json_object"} {
		if redistributeContainsString(result, mode) || redistributeContainsString(result, "decisions") {
			continue
		}
		result = append(result, mode)
	}
	return result
}

// nonEmptyModalities returns actual when present and otherwise returns a copy
// of fallback.
func nonEmptyModalities(actual, fallback []string) []string {
	if len(actual) == 0 {
		return append([]string(nil), fallback...)
	}
	return append([]string(nil), actual...)
}

// redistributeContainsString reports whether values contains wanted.
func redistributeContainsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
