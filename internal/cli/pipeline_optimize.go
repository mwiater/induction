package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	induction "github.com/mwiater/induction"
	"github.com/mwiater/induction/internal/pipelineoptimizer"
	"github.com/spf13/cobra"
)

type optimizerAdvisor struct {
	client  *induction.Client
	timeout time.Duration
}

func (a optimizerAdvisor) Infer(ctx context.Context, model, system, user string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	maxTokens := 12000
	request := &induction.ChatRequest{Model: model, Messages: []induction.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}, ResponseFormat: &induction.ResponseFormat{Type: "json_object"}, MaxTokens: &maxTokens}
	snapshot, err := a.client.GenerateSnapshot(ctx, request)
	if err != nil {
		return "", err
	}
	if snapshot == nil || len(snapshot.Interaction) == 0 {
		return "", fmt.Errorf("optimizer model returned no response")
	}
	return snapshot.Interaction[0].Content, nil
}

func newOptimizePipelineCommand(configPath *string) *cobra.Command {
	var o pipelineoptimizer.Options
	var sessions string
	cmd := &cobra.Command{
		Use: "optimize", Short: "propose a statically validated alternative pipeline", Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if sessions != "auto" && sessions != "none" {
				return fmt.Errorf("--sessions must be auto or none")
			}
			o.AutoSessions = sessions == "auto" && (cmd.Flags().Changed("sessions") || len(o.SessionIDs)+len(o.SessionFiles) == 0)
			if o.Verbose {
				o.Progress = func(message string) { _, _ = fmt.Fprintln(cmd.OutOrStdout(), message) }
			}
			cfg, err := induction.LoadConfig(*configPath)
			if err != nil {
				return err
			}
			timeout := time.Duration(cfg.Timeout)
			if timeout <= 0 || timeout > 5*time.Minute {
				timeout = 5 * time.Minute
			}
			service := pipelineoptimizer.Service{Advisor: optimizerAdvisor{client: induction.NewClient(cmd.Context(), cfg.Server), timeout: timeout}}
			result, err := service.Optimize(cmd.Context(), o)
			if err != nil {
				return err
			}
			if o.DryRun {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Static inspection passed; optimizer inference and candidate generation were skipped. Completed sessions used: %d\n\n%s\n\n%s\n", result.SessionCount, result.PreviewYAML, result.Report)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Alternative YAML: %s\nAnalysis: %s\nCompleted sessions used: %d\nStatic validation: passed\n", result.YAMLPath, result.ReportPath, result.SessionCount)
			}
			if o.Verbose && len(result.Warnings) > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Warnings: "+strings.Join(result.Warnings, "; "))
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Not executed or performance-tested; run manually to evaluate.")
			return nil
		},
	}
	cmd.Flags().StringVar(&o.PipelinePath, "pipeline", "", "existing pipeline YAML path (required)")
	cmd.Flags().StringVar(&sessions, "sessions", "auto", "automatic completed-session discovery: auto or none")
	cmd.Flags().StringArrayVar(&o.SessionIDs, "session", nil, "completed session ID (repeatable; overrides auto selection)")
	cmd.Flags().StringArrayVar(&o.SessionFiles, "session-file", nil, "completed exported session JSON (repeatable)")
	cmd.Flags().IntVar(&o.MaxSessions, "max-sessions", 10, "maximum completed sessions to use (1-50)")
	cmd.Flags().StringVar(&o.SourceRoot, "source-root", "", "matching Induction source checkout (default current directory)")
	cmd.Flags().StringVar(&o.OutputDir, "output-dir", "", "existing directory for alternatives (default pipeline directory)")
	cmd.Flags().StringVar(&o.Model, "model", "", "local optimizer model ID (default first pipeline model)")
	cmd.Flags().StringVar(&o.Focus, "focus", "balanced", "balanced, reliability, efficiency, or structure")
	cmd.Flags().BoolVar(&o.DryRun, "dry-run", false, "preview validated YAML and analysis without creating files")
	cmd.Flags().BoolVar(&o.Verbose, "verbose", false, "show evidence warnings")
	return cmd
}
