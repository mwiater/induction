// Package cli defines Induction's command-line interface.
package cli

import (
	"fmt"
	"os/exec"
	"strings"

	induction "github.com/mwiater/induction"
	"github.com/spf13/cobra"
)

const (
	// siaQuestionsPath identifies a fixed value used internally by this package.
	siaQuestionsPath = "data/sia/shuffle_questions_320.jsonl"
	// siaOutputPath identifies a fixed value used internally by this package.
	siaOutputPath = "data/dashboard/sia.json"
)

// newRunCommand contains beta and experimental workflows that are not part of
// the normal inference command surface.
func newRunCommand(configPath *string) *cobra.Command {
	run := &cobra.Command{
		Use:   "run",
		Short: "run an experimental workflow",
		Args:  cobra.NoArgs,
	}
	run.AddCommand(newSIARunCommand(configPath))
	return run
}

// newSIARunCommand performs the internal operation described by its signature.
func newSIARunCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "sia",
		Short: "run the beta shuffle-invariance evaluation",
		Long:  "Run the beta shuffle-invariance assessment (SIA) against every model exposed by the configured llama.cpp server.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := induction.LoadConfig(*configPath)
			if err != nil {
				return err
			}
			if strings.TrimSpace(cfg.Server) == "" {
				return fmt.Errorf("induction.yaml server is required for SIA")
			}
			args := siaCommandArgs(cfg.Server)
			process := exec.CommandContext(cmd.Context(), "go", args...)
			process.Stdout = cmd.OutOrStdout()
			process.Stderr = cmd.ErrOrStderr()
			if err := process.Run(); err != nil {
				if ctxErr := cmd.Context().Err(); ctxErr != nil {
					return ctxErr
				}
				return fmt.Errorf("run SIA evaluator: %w", err)
			}
			return nil
		},
	}
}

// siaCommandArgs performs the internal operation described by its signature.
func siaCommandArgs(server string) []string {
	return []string{
		"run", "./internal/sia/eval.go",
		"--url", server,
		"--questions", siaQuestionsPath,
		"--models-all",
		"--nprobs", "100",
		"--out", siaOutputPath,
		"--resume",
		"--resume-from", ".sandbox/shuffle-results.json",
	}
}
