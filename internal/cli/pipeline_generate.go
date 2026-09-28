package cli

import (
	"fmt"
	"os"
	"strings"

	induction "github.com/mwiater/induction"
	"github.com/mwiater/induction/internal/pipelinegen"
	"github.com/spf13/cobra"
)

func newPipelineCommand(configPath *string) *cobra.Command {
	pipeline := &cobra.Command{Use: "pipeline", Short: "create and work with pipelines"}
	var model, prompt, promptFile, output string
	var force, validate bool
	generate := &cobra.Command{
		Use: "generate", Short: "generate a runnable pipeline from a complex prompt", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(model) == "" {
				return fmt.Errorf("--model is required")
			}
			if (strings.TrimSpace(prompt) == "") == (strings.TrimSpace(promptFile) == "") {
				return fmt.Errorf("exactly one of --prompt or --prompt-file is required")
			}
			if strings.TrimSpace(output) == "" {
				return fmt.Errorf("--output is required")
			}
			original := prompt
			if promptFile != "" {
				data, err := os.ReadFile(promptFile)
				if err != nil {
					return fmt.Errorf("read prompt file: %w", err)
				}
				original = string(data)
			}
			original = strings.TrimSpace(original)
			if original == "" {
				return fmt.Errorf("prompt is empty")
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Analyzing prompt...")
			var plannerInteraction *induction.Interaction
			plannerPipeline := &induction.Pipeline{Name: "pipeline-generator", Steps: []induction.PipelineStep{{
				Name: "plan", Model: model, SystemPrompt: pipelinegen.PlannerSystemPrompt,
				UserPrompt:     "[ORIGINAL USER PROMPT]\n<USER_PROMPT>\n" + original + "\n</USER_PROMPT>",
				ResponseFormat: &induction.ResponseFormat{Type: "json_object"},
			}}}
			plannerReq := &induction.ChatRequest{Model: model}
			if err := runConfiguredChat(cmd.Context(), plannerReq, cmd.InOrStdin(), cmd.OutOrStdout(),
				induction.WithConfigPath(*configPath),
				induction.WithPipeline(plannerPipeline),
				induction.WithAutoExitAfterInitialChat(true),
				induction.WithInferenceCompleted(func(snapshot *induction.ModelSnapshot, _ string) {
					if snapshot != nil && len(snapshot.Interaction) > 0 {
						plannerInteraction = &snapshot.Interaction[0]
					}
				}),
			); err != nil {
				return err
			}
			if plannerInteraction == nil {
				return fmt.Errorf("planner returned no interaction")
			}
			plan, err := pipelinegen.ParsePlanResponse(plannerInteraction)
			if err != nil {
				return err
			}
			if err := pipelinegen.ValidatePlan(plan); err != nil {
				return err
			}
			var planPipeline *induction.Pipeline
			if plan.DecompositionRecommended {
				planPipeline, err = pipelinegen.Compile(plan, model, original, validate)
				if err != nil {
					return err
				}
			}
			if planPipeline == nil {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nPrompt classification: %s\nDecomposition recommended: no\n\nNo pipeline generated.\nUse the existing prompt optimizer for this request.\n", plan.Classification)
				return nil
			}
			if err := pipelinegen.WritePipeline(output, planPipeline, force); err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nPrompt classification: %s\nDecomposition recommended: yes\nSubtasks: %d\nFinal synthesis: enabled\nValidation: %s\n\nGenerated pipeline:\n  %s\n\nSteps:\n", plan.Classification, len(plan.Tasks), boolWord(validate), output)
			for i, step := range planPipeline.Steps {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "  %d. %s\n", i+1, step.Name)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nRun with:\n  induction --pipeline %s\n", output)
			return nil
		},
	}
	generate.Flags().StringVar(&model, "model", "", "model ID to use for planning and generated steps")
	generate.Flags().StringVar(&prompt, "prompt", "", "original user prompt")
	generate.Flags().StringVar(&promptFile, "prompt-file", "", "file containing the original user prompt")
	generate.Flags().StringVar(&output, "output", "", "output pipeline YAML path")
	generate.Flags().BoolVar(&force, "force", false, "overwrite an existing output file")
	generate.Flags().BoolVar(&validate, "validate", false, "append a read-only final validation step")
	pipeline.AddCommand(generate)
	return pipeline
}

func boolWord(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}
