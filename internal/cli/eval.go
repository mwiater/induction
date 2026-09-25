package cli

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	induction "github.com/mwiater/induction"
	"github.com/mwiater/induction/internal/eval"
	"github.com/mwiater/induction/internal/eval/inspect"
	"github.com/spf13/cobra"
)

func newEvalCommand(configPath *string) *cobra.Command {
	var evalConfig, model string
	var allModels bool
	cmd := &cobra.Command{
		Use: "eval", Short: "run a configured local model evaluation", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(evalConfig) == "" {
				return fmt.Errorf("--eval-config is required")
			}
			if !allModels && strings.TrimSpace(model) == "" {
				return fmt.Errorf("--model is required")
			}
			if allModels && strings.TrimSpace(model) != "" {
				return fmt.Errorf("--model cannot be combined with --allModels")
			}
			suite, err := eval.LoadConfig(evalConfig)
			if err != nil {
				return err
			}
			cfg, err := induction.LoadConfig(*configPath)
			if err != nil {
				return err
			}
			if allModels {
				client := induction.NewClient(cmd.Context(), cfg.Server)
				server, err := client.InspectServer(cmd.Context())
				if err != nil {
					return fmt.Errorf("list models on configured llama.cpp server: %w", err)
				}
				if len(server.Models) == 0 {
					return fmt.Errorf("no models are available on the configured llama.cpp server")
				}
				var failures, runnable int
				for _, runtimeModel := range server.Models {
					if strings.TrimSpace(runtimeModel.ID) == "" || runtimeModel.Failed {
						continue
					}
					runnable++
					if err := runEvalForModel(cmd, cfg, suite, runtimeModel.ID); err != nil {
						failures++
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Model %s failed: %v\n", runtimeModel.ID, err)
					}
				}
				if runnable == 0 {
					return fmt.Errorf("no usable models are available on the configured llama.cpp server")
				}
				if failures > 0 {
					return fmt.Errorf("%d model evaluation(s) failed", failures)
				}
				return nil
			}
			return runEvalForModel(cmd, cfg, suite, model)
		},
	}
	cmd.Flags().StringVar(&evalConfig, "eval-config", "", "evaluation suite YAML configuration")
	cmd.Flags().StringVar(&model, "model", "", "model ID to evaluate")
	cmd.Flags().BoolVar(&allModels, "allModels", false, "run the evaluation suite against every available model")
	cmd.AddCommand(newEvalStatusCommand())
	return cmd
}

func newEvalStatusCommand() *cobra.Command {
	var evalConfig, model string
	status := &cobra.Command{
		Use:   "status",
		Short: "show evaluation completion status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(evalConfig) == "" {
				return fmt.Errorf("--eval-config is required")
			}
			if strings.TrimSpace(model) == "" {
				return fmt.Errorf("--model is required")
			}
			suite, err := eval.LoadConfig(evalConfig)
			if err != nil {
				return err
			}
			result, err := eval.LoadResult(".", model, suite.Name)
			if err != nil {
				return fmt.Errorf("load evaluation result: %w", err)
			}
			return renderEvalStatus(cmd.OutOrStdout(), suite, model, result)
		},
	}
	status.Flags().StringVar(&evalConfig, "eval-config", "", "evaluation suite YAML configuration")
	status.Flags().StringVar(&model, "model", "", "model ID to inspect")
	return status
}

func renderEvalStatus(out io.Writer, suite *eval.Config, model string, result *eval.Result) error {
	benchmarks := eval.Status(suite, result)
	complete := 0
	for _, benchmark := range benchmarks {
		if benchmark.Complete {
			complete++
		}
	}

	_, _ = fmt.Fprintf(out, "Evaluation Status\n\nSuite: %s\nModel: %s\n\n", suite.Name, model)
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "EVAL\tSTATUS\tSAMPLES\tREQUIRED")
	for _, benchmark := range benchmarks {
		state := "missing"
		if benchmark.Complete {
			state = "complete"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%d\t%d\n", benchmark.Definition.Name, state, benchmark.Samples, benchmark.Definition.Limit)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	missing := len(benchmarks) - complete
	_, _ = fmt.Fprintf(out, "\nComplete: %d/%d\n", complete, len(benchmarks))
	if missing > 0 {
		_, _ = fmt.Fprintf(out, "Missing: %d\n", missing)
	}
	return nil
}

func runEvalForModel(cmd *cobra.Command, cfg *induction.Config, suite *eval.Config, model string) error {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Induction Eval")
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nSuite:  %s\nModel:  %s\nServer: %s\n\n", suite.Name, model, cfg.Server)
	result, runErr := eval.Run(cmd.Context(), cfg, suite, model, ".", cmd.OutOrStdout(), inspect.Runner{}, eval.RunOptions{OnBenchmarkComplete: func(completion eval.BenchmarkCompletion) {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Benchmark %s: %d samples, %.2f%% | completed in: %s\n", completion.Result.Name, completion.Result.Samples, completion.Result.Score*100, completion.Duration.Round(time.Millisecond))
	}})
	if result != nil {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Duration: %.2fs\n", float64(result.DurationMS)/1000)
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nEvaluation %s\n", result.Status)
		if result.Aggregate != nil {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Aggregate: %.2f%%\n", result.Aggregate.Score*100)
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Result: %s\nRaw Inspect logs: %s\n", filepath.Join("data", "evals", "results", eval.SafeModelName(model), suite.Name+".json"), filepath.Join("data", "evals", "raw", result.RunID))
	}
	return runErr
}
