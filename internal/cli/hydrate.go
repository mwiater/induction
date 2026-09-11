package cli

import (
	"fmt"
	"os"
	"os/exec"

	induction "github.com/mwiater/induction"
	"github.com/spf13/cobra"
)

type hydrationWorkload struct {
	name  string
	flags inferenceFlags
}

type hydrationSummary struct {
	Models    int
	Scheduled int
	Succeeded int
	Failed    int
	Skipped   int
}

func newSessionsCommand(configPath *string) *cobra.Command {
	sessions := &cobra.Command{Use: "sessions", Short: "manage persisted inference sessions"}
	sessions.AddCommand(newHydrateCommand(configPath))
	return sessions
}

func newHydrateCommand(configPath *string) *cobra.Command {
	hydrate := &cobra.Command{
		Use:   "hydrate",
		Short: "populate sessions with telemetry across installed models",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runHydrate(cmd, *configPath)
		},
	}
	return hydrate
}

func runHydrate(cmd *cobra.Command, configPath string) error {
	cfg, err := induction.LoadConfig(configPath)
	if err != nil {
		return err
	}
	client, err := configuredClient(cmd, configPath)
	if err != nil {
		return err
	}
	catalog, err := client.ListModelCatalog(cmd.Context())
	if err != nil {
		return fmt.Errorf("list models from /v1/models: %w", err)
	}

	if len(catalog) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No models were reported by /v1/models; generating dashboard from existing sessions.")
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Hydrating %d server-reported model(s).\n", len(catalog))
	}

	mcpConfigured := len(cfg.MCPServers) > 0
	if !mcpConfigured {
		fmt.Fprintln(cmd.OutOrStdout(), "MCP workload skipped: no MCP servers are configured.")
	}

	summary := hydrationSummary{Models: len(catalog)}
	for _, entry := range catalog {
		model := entry.ID
		vision := entry.Capabilities.ImageInput

		workloads := hydrationWorkloads(model, configPath, vision, mcpConfigured)
		for _, workload := range workloads {
			summary.Scheduled++
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s…\n", model, workload.name)
			if err := runHydrationCommand(cmd, configPath, workload.args()); err != nil {
				summary.Failed++
				fmt.Fprintf(cmd.OutOrStdout(), "[%s] %s failed: %v\n", model, workload.name, err)
				continue
			}
			summary.Succeeded++
		}
		if !vision {
			summary.Skipped++
			fmt.Fprintf(cmd.OutOrStdout(), "[%s] image workload skipped: model does not report image input.\n", model)
		}
	}

	fmt.Fprintln(cmd.OutOrStdout(), "Running dashboard generation…")
	if err := runHydrationCommand(cmd, configPath, []string{"dashboard", "generate"}); err != nil {
		return fmt.Errorf("dashboard generate after hydration: %w", err)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "Hydration complete: models=%d scheduled=%d succeeded=%d failed=%d image_workloads_skipped=%d\n", summary.Models, summary.Scheduled, summary.Succeeded, summary.Failed, summary.Skipped)
	return nil
}

func runHydrationCommand(parent *cobra.Command, configPath string, args []string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate induction executable: %w", err)
	}
	childArgs := make([]string, 0, len(args)+2)
	if configPath != "" {
		childArgs = append(childArgs, "--config", configPath)
	}
	childArgs = append(childArgs, args...)
	child := exec.CommandContext(parent.Context(), executable, childArgs...)
	child.Stdin = parent.InOrStdin()
	child.Stdout = parent.OutOrStdout()
	child.Stderr = parent.ErrOrStderr()
	if err := child.Run(); err != nil {
		return err
	}
	return nil
}

func (w hydrationWorkload) args() []string {
	args := []string{"--model", w.flags.model, "--userPrompt", w.flags.userPrompt}
	if w.flags.image != "" {
		args = append(args, "--image", w.flags.image)
	}
	if w.flags.document != "" {
		args = append(args, "--document", w.flags.document)
	}
	if w.flags.nomcp {
		args = append(args, "--nomcp")
	}
	args = append(args, "--autosubmit", "--autoexit")
	return args
}

func hydrationWorkloads(model, configPath string, vision, mcpConfigured bool) []hydrationWorkload {
	base := func(name, prompt string, nomcp bool) hydrationWorkload {
		return hydrationWorkload{name: name, flags: inferenceFlags{config: configPath, model: model, userPrompt: prompt, autosubmit: true, autoexit: true, nomcp: nomcp}}
	}
	workloads := []hydrationWorkload{
		base("text", "Explain the key idea in this short telemetry hydration exercise in three concise paragraphs.", true),
		{name: "document", flags: inferenceFlags{config: configPath, model: model, userPrompt: "Summarize the provided document, including its main argument, evidence, and conclusion.", document: "data/fixtures/documents/fixture.pdf", autosubmit: true, autoexit: true, nomcp: true}},
		{name: "application tools", flags: inferenceFlags{config: configPath, model: model, userPrompt: "Use the available application tools to report the current date and time, available RAM, and free disk space on the root filesystem.", autosubmit: true, autoexit: true, nomcp: true}},
	}
	if mcpConfigured {
		workloads = append(workloads, base("MCP tools", "Use an available read-only MCP tool and report the result.", false))
	}
	if vision {
		workloads = append(workloads, hydrationWorkload{name: "image", flags: inferenceFlags{config: configPath, model: model, userPrompt: "Analyze the provided image in detail, including its subjects, composition, lighting, colors, and context.", image: "data/fixtures/images/fixture.jpg", autosubmit: true, autoexit: true, nomcp: true}})
	}
	return workloads
}
