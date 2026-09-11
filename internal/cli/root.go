// Package cli implements the induction command-line interface.
package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	induction "github.com/mwiater/induction"
	"github.com/mwiater/induction/internal/modelmanager"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// NewRootCommand constructs the induction CLI.
func NewRootCommand() *cobra.Command {
	var configPath string
	var inference inferenceFlags
	root := &cobra.Command{Use: "induction", Short: "run inference and manage Induction", SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			inference.parameterSet = map[string]bool{}
			for _, name := range []string{"temperature", "top-p", "top-k", "max-tokens", "repeat-penalty", "seed"} {
				inference.parameterSet[name] = cmd.Flags().Changed(name)
			}
			inference.parameterOverrides = false
			for _, set := range inference.parameterSet {
				if set {
					inference.parameterOverrides = true
					break
				}
			}
			if inference.model == "" && inference.pipeline == "" && inference.image == "" && inference.document == "" && inference.userPrompt == "" && inference.systemPrompt == "" && inference.responseFormat == "" && inference.jsonSchema == "" && !inference.parameterOverrides {
				return cmd.Help()
			}
			inference.config = configPath
			return runInference(cmd.Context(), inference, cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	root.PersistentFlags().StringVar(&configPath, "config", "induction.yaml", "configuration file")
	addInferenceFlags(root, &inference)
	models := newModelManagerCommand(&configPath)
	root.AddCommand(models)
	root.AddCommand(newListCommand(root))
	root.AddCommand(newRuntimeCommand(&configPath))
	server, modelInspect := newInspectCommands(&configPath)
	root.AddCommand(server)
	root.AddCommand(newDashboardCommand())
	root.AddCommand(newUICommand())
	sessions := newSessionsCommand(&configPath)
	sessions.AddCommand(newCleanSessionsCommand())
	sessions.AddCommand(newInspectSessionCommand())
	root.AddCommand(sessions)
	models.AddCommand(modelInspect)
	return root
}

// commandInfo holds the command path and short description used by the
// commands listing.
type commandInfo struct {
	path        string
	description string
}

func newListCommand(root *cobra.Command) *cobra.Command {
	commands := &cobra.Command{
		Use:   "commands",
		Short: "list all commands and subcommands",
		Long:  "The 'commands' subcommand lists all commands and subcommands in a hierarchical, indented format, with the command path in the first column and its short description in the second column.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			commandData := collectCommandData(root, "", "")
			maxPathLength := 0
			for _, data := range commandData {
				if strings.Contains(data.path, "completion") {
					continue
				}
				if len(data.path) > maxPathLength {
					maxPathLength = len(data.path)
				}
			}

			out := cmd.OutOrStdout()
			fmt.Fprintln(out, "Commands and Subcommands:")
			for _, data := range commandData {
				if strings.Contains(data.path, "completion") {
					continue
				}
				fmt.Fprintf(out, "  %s%s%s\n", data.path, strings.Repeat(" ", maxPathLength-len(data.path)+2), data.description)
			}
			return nil
		},
	}

	list := &cobra.Command{Use: "list", Short: "list available command groups"}
	list.AddCommand(commands)
	return list
}

// collectCommandData recursively flattens the command tree into display rows.
func collectCommandData(cmd *cobra.Command, currentPath, indent string) []commandInfo {
	fullPath := currentPath + cmd.Name()
	if currentPath != "" {
		fullPath = currentPath + " " + cmd.Name()
	}

	data := []commandInfo{{path: indent + fullPath, description: cmd.Short}}
	for _, subCmd := range cmd.Commands() {
		data = append(data, collectCommandData(subCmd, fullPath, indent+"  ")...)
	}
	return data
}

func newCleanSessionsCommand() *cobra.Command {
	return &cobra.Command{
		Use: "clean", Args: cobra.NoArgs,
		Short: "remove invalid persisted sessions",
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := induction.CleanSessions("")
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Sessions scanned: %d\nSessions deleted: %d\nInvalid sessions deleted: %d\nNull-snapshot sessions deleted: %d\n", result.Scanned, result.Deleted, result.InvalidDeleted, result.NullSnapshotsDeleted)
			return nil
		},
	}
}

func newUICommand() *cobra.Command {
	ui := &cobra.Command{Use: "ui", Short: "preview console UI options"}
	theme := &cobra.Command{
		Use:   "theme",
		Args:  cobra.NoArgs,
		Short: "preview console UI theme colors and styles",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return induction.RunConsoleThemePreview(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout())
		},
	}
	ui.AddCommand(theme)
	return ui
}

func configuredClient(cmd *cobra.Command, configPath string) (*induction.Client, error) {
	cfg, err := induction.LoadConfig(configPath)
	if err != nil {
		return nil, err
	}
	return induction.NewClient(cmd.Context(), cfg.Server,
		induction.WithLoadWaitInterval(time.Duration(cfg.LoadWaitInterval)),
		induction.WithHTTPClient(&http.Client{Timeout: time.Duration(cfg.Timeout)}),
		induction.WithLogger(induction.NewConfiguredLogger(cfg.Log))), nil
}

func newInspectCommands(configPath *string) (*cobra.Command, *cobra.Command) {
	serverInspect := &cobra.Command{Use: "inspect", Short: "inspect the configured inference server", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := configuredClient(cmd, *configPath)
		if err != nil {
			return err
		}
		result, err := client.InspectServer(cmd.Context())
		if err != nil {
			return err
		}
		if outputJSON, _ := cmd.Flags().GetBool("json"); outputJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		return renderServerInspection(cmd.OutOrStdout(), result)
	}}
	serverInspect.Flags().Bool("json", false, "write JSON output")
	modelInspect := &cobra.Command{Use: "inspect MODEL", Short: "inspect a model's capabilities and runtime", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		client, err := configuredClient(cmd, *configPath)
		if err != nil {
			return err
		}
		result, err := client.InspectModel(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if outputJSON, _ := cmd.Flags().GetBool("json"); outputJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		return renderModelInspection(cmd.OutOrStdout(), result)
	}}
	modelInspect.Flags().Bool("json", false, "write JSON output")
	server := &cobra.Command{Use: "server", Short: "inspect the configured inference server"}
	server.AddCommand(serverInspect)
	return server, modelInspect
}

func newInspectSessionCommand() *cobra.Command {
	var sessionPath string
	session := &cobra.Command{
		Use:   "inspect",
		Short: "replay a persisted chat session as an instant transcript",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if strings.TrimSpace(sessionPath) == "" {
				return fmt.Errorf("--session is required")
			}
			chatSession, err := induction.LoadChatSession(sessionPath)
			if err != nil {
				return err
			}
			return induction.RenderSessionTranscript(cmd.OutOrStdout(), chatSession)
		},
	}
	session.Flags().StringVar(&sessionPath, "session", "", "path to the persisted session file")
	return session
}

func renderServerInspection(out io.Writer, result *induction.ServerInspection) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "Server")
	fmt.Fprintf(w, "  Endpoint\t%s\n  Health\t%s\n  Role\t%s\n  Models\t%d\n  Loaded\t%d\n", result.Endpoint, boolLabel(result.Healthy), result.Role, len(result.Models), len(result.LoadedModels))
	if len(result.LoadedModels) > 0 {
		fmt.Fprintln(w, "\nLoaded Models")
		for _, model := range result.LoadedModels {
			fmt.Fprintf(w, "  %s\n", model)
		}
	}
	return w.Flush()
}

func renderModelInspection(out io.Writer, result *induction.ModelInspection) error {
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "Model")
	fmt.Fprintf(w, "  ID\t%s\n  State\t%s\n  Path\t%s\n\nCapabilities\n  Text input\t%s\n  Image input\t%s\n  Audio input\t%s\n  Text output\t%s\n\nRuntime\n", result.ID, result.State, valueOrUnknown(result.Path), boolLabel(result.Capabilities.TextInput), boolLabel(result.Capabilities.ImageInput), boolLabel(result.Capabilities.AudioInput), boolLabel(result.Capabilities.TextOutput))
	r := result.Runtime
	if r.ContextSize != nil {
		fmt.Fprintf(w, "  Context\t%d\n", *r.ContextSize)
	} else {
		fmt.Fprintln(w, "  Context\tunknown")
	}
	if r.BatchSize != nil {
		fmt.Fprintf(w, "  Batch\t%d\n", *r.BatchSize)
	} else {
		fmt.Fprintln(w, "  Batch\tunknown")
	}
	if r.UBatchSize != nil {
		fmt.Fprintf(w, "  UBatch\t%d\n", *r.UBatchSize)
	} else {
		fmt.Fprintln(w, "  UBatch\tunknown")
	}
	if r.Parallel != nil {
		fmt.Fprintf(w, "  Parallel\t%d\n", *r.Parallel)
	} else {
		fmt.Fprintln(w, "  Parallel\tunknown")
	}
	fmt.Fprintf(w, "  Cache K\t%s\n  Cache V\t%s\n", valueOrUnknown(r.CacheTypeK), valueOrUnknown(r.CacheTypeV))
	if r.FlashAttention == nil {
		fmt.Fprintln(w, "  Flash Attn\tunknown")
	} else {
		fmt.Fprintf(w, "  Flash Attn\t%s\n", boolLabel(*r.FlashAttention))
	}
	return w.Flush()
}

func boolLabel(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}
func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func newRuntimeCommand(configPath *string) *cobra.Command {
	var jsonOutput bool
	clientFor := func(cmd *cobra.Command) (*induction.Client, error) {
		return configuredClient(cmd, *configPath)
	}
	status := &cobra.Command{Use: "status", Short: "show the server's runtime model state", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
		client, err := clientFor(cmd)
		if err != nil {
			return err
		}
		result, err := client.GetRuntimeStatus(cmd.Context())
		if err != nil {
			return err
		}
		if jsonOutput {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 4, 2, ' ', 0)
		fmt.Fprintln(writer, "MODEL\tSTATE\tFAILED")
		for _, model := range result.Models {
			fmt.Fprintf(writer, "%s\t%s\t%t\n", model.ID, model.State, model.Failed)
		}
		return writer.Flush()
	}}
	status.Flags().BoolVar(&jsonOutput, "json", false, "write JSON output")
	operation := func(name string, action func(*induction.Client, context.Context, string) (*induction.RuntimeOperation, error)) *cobra.Command {
		var outputJSON bool
		cmd := &cobra.Command{Use: name + " MODEL", Short: strings.ToLower(name) + " a runtime model", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
			client, err := clientFor(cmd)
			if err != nil {
				return err
			}
			op, err := action(client, cmd.Context(), args[0])
			if outputJSON {
				if op != nil {
					_ = json.NewEncoder(cmd.OutOrStdout()).Encode(op)
				}
				return err
			}
			if err != nil {
				return err
			}
			if !op.Changed {
				fmt.Fprintf(cmd.OutOrStdout(), "%s is already %s.\n", op.Model, op.To)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s in %s.\n", strings.Title(name), op.Model, op.Duration.Round(time.Millisecond))
			return nil
		}}
		cmd.Flags().BoolVar(&outputJSON, "json", false, "write JSON output")
		return cmd
	}
	root := &cobra.Command{Use: "runtime", Short: "manage the server's runtime models"}
	root.AddCommand(status, operation("load", func(c *induction.Client, ctx context.Context, model string) (*induction.RuntimeOperation, error) {
		return c.LoadModel(ctx, model)
	}), operation("unload", func(c *induction.Client, ctx context.Context, model string) (*induction.RuntimeOperation, error) {
		return c.UnloadModel(ctx, model)
	}))
	var switchJSON bool
	switchCmd := &cobra.Command{Use: "switch MODEL", Short: "switch the active runtime model", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		client, err := clientFor(cmd)
		if err != nil {
			return err
		}
		result, err := client.SwitchModel(cmd.Context(), args[0])
		if switchJSON {
			if result != nil {
				_ = json.NewEncoder(cmd.OutOrStdout()).Encode(result)
			}
			return err
		}
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Switched runtime to %s in %s.\n", result.Target, result.Duration.Round(time.Millisecond))
		return nil
	}}
	switchCmd.Flags().BoolVar(&switchJSON, "json", false, "write JSON output")
	root.AddCommand(switchCmd)
	return root
}

func newModelManagerCommand(configPath *string) *cobra.Command {
	var modelsPath string
	var searchResults int
	var providers []string
	cmd := &cobra.Command{
		Use:     "models [initial-query]",
		Aliases: []string{"model-manager"},
		Short:   "search, download, and manage models",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadModelManagerConfig(*configPath, cmd, modelsPath, searchResults, providers)
			if err != nil {
				return err
			}
			client, err := modelmanager.NewHFCLIClient(cfg.HuggingFaceToken)
			if err != nil {
				return err
			}
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			return modelmanager.RunInteractive(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), client, interactiveOptions(cfg, client), query)
		},
	}
	cmd.PersistentFlags().StringVar(&modelsPath, "models-path", "", "directory used to store models")
	cmd.PersistentFlags().IntVar(&searchResults, "search-results", 0, "maximum search results")
	cmd.PersistentFlags().StringSliceVar(&providers, "preferred-provider", nil, "preferred provider (repeatable)")
	cmd.AddCommand(newSearchCommand(configPath, func() (string, int, []string) { return modelsPath, searchResults, providers }))
	cmd.AddCommand(newFilesCommand(configPath, func() (string, int, []string) { return modelsPath, searchResults, providers }))
	cmd.AddCommand(newDownloadCommand(configPath, func() (string, int, []string) { return modelsPath, searchResults, providers }))
	cmd.AddCommand(newInstalledCommands(configPath, func() (string, int, []string) { return modelsPath, searchResults, providers })...)
	cmd.AddCommand(newUpdateCommand(configPath, func() (string, int, []string) { return modelsPath, searchResults, providers }))
	cmd.AddCommand(newEvalsCommand(configPath, func() (string, int, []string) { return modelsPath, searchResults, providers }))
	cmd.AddCommand(newMMProjCommand(configPath, func() (string, int, []string) { return modelsPath, searchResults, providers }))
	return cmd
}

func newMMProjCommand(configPath *string, inherited func() (string, int, []string)) *cobra.Command {
	var yes, outputJSON bool
	command := &cobra.Command{
		Use:   "mmproj",
		Short: "check installed vision models for downloaded mmproj files",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			modelsPath, searchResults, providers := inherited()
			cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
			if err != nil {
				return err
			}
			index, err := modelmanager.BuildInstalledIndex(cfg.ModelsPath)
			if err != nil {
				return err
			}
			client, err := modelmanager.NewHFCLIClient(cfg.HuggingFaceToken)
			if err != nil {
				return err
			}
			input := bufio.NewReader(cmd.InOrStdin())

			type result struct {
				Repository string                   `json:"repository"`
				Vision     bool                     `json:"vision"`
				Files      []modelmanager.ModelFile `json:"mmprojFiles,omitempty"`
				Missing    []string                 `json:"missing,omitempty"`
				Downloaded []string                 `json:"downloaded,omitempty"`
				Error      string                   `json:"error,omitempty"`
			}
			results := make([]result, 0)
			failed := 0
			for _, repository := range modelmanager.InstalledRepositories(index) {
				item := result{Repository: repository}
				revision, files, listErr := client.ListFiles(cmd.Context(), repository)
				if listErr != nil {
					item.Error = listErr.Error()
					failed++
					results = append(results, item)
					continue
				}
				mmprojs := modelmanager.MMProjFiles(files)
				item.Files = mmprojs
				item.Vision = len(mmprojs) > 0
				if !item.Vision {
					results = append(results, item)
					continue
				}
				missing := make([]modelmanager.ModelFile, 0, len(mmprojs))
				for _, file := range mmprojs {
					if modelmanager.MMProjInstalled(cfg.ModelsPath, repository, file.Path) {
						item.Downloaded = append(item.Downloaded, file.Path)
						continue
					}
					item.Missing = append(item.Missing, file.Path)
					missing = append(missing, file)
				}
				if !yes && len(missing) > 1 {
					fmt.Fprintf(cmd.ErrOrStderr(), "Missing mmproj files for %s:\n", repository)
					for i, file := range missing {
						fmt.Fprintf(cmd.ErrOrStderr(), "  %d) %s\n", i+1, file.Path)
					}
					fmt.Fprint(cmd.ErrOrStderr(), "Choose an mmproj file to download [1-", len(missing), ", Enter to skip]: ")
					answer, readErr := input.ReadString('\n')
					if readErr != nil && len(answer) == 0 {
						missing = nil
					} else {
						choice, parseErr := strconv.Atoi(strings.TrimSpace(answer))
						if parseErr != nil || choice < 1 || choice > len(missing) {
							missing = nil
						} else {
							missing = []modelmanager.ModelFile{missing[choice-1]}
						}
					}
				} else if !yes && len(missing) == 1 {
					file := missing[0]
					fmt.Fprintf(cmd.ErrOrStderr(), "Download mmproj %s for %s? [y/N] ", file.Path, repository)
					answer, readErr := input.ReadString('\n')
					if (readErr != nil && len(answer) == 0) || (strings.ToLower(strings.TrimSpace(answer)) != "y" && strings.ToLower(strings.TrimSpace(answer)) != "yes") {
						missing = nil
					}
				}
				for _, file := range missing {
					fmt.Fprintf(cmd.ErrOrStderr(), "[mmproj] downloading %s/%s\n", repository, file.Path)
					// The artifact was confirmed missing above. Overwrite also permits
					// replacing the stale manifest left behind by a manual cleanup.
					_, downloadErr := modelmanager.Download(cmd.Context(), client.Path, modelmanager.DownloadRequest{Repository: repository, File: file.Path, Revision: revision, ModelsPath: cfg.ModelsPath, Size: file.Size, ETag: file.ETag, LFSOID: file.LFSOID, Overwrite: true, Token: cfg.HuggingFaceToken})
					if downloadErr != nil {
						item.Error = downloadErr.Error()
						failed++
						continue
					}
					item.Downloaded = append(item.Downloaded, file.Path)
				}
				results = append(results, item)
			}
			if outputJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(results)
			}
			for _, item := range results {
				status := "text-only"
				if item.Vision {
					status = fmt.Sprintf("vision (%d/%d mmproj files downloaded)", len(item.Downloaded), len(item.Files))
				}
				if item.Error != "" {
					status = "ERROR: " + item.Error
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s  %s\n", item.Repository, status)
			}
			if failed > 0 {
				return fmt.Errorf("mmproj check failed for %d operation(s)", failed)
			}
			return nil
		},
	}
	command.Flags().BoolVar(&yes, "yes", false, "download all missing mmproj files without prompting")
	command.Flags().BoolVar(&outputJSON, "json", false, "write JSON output")
	return command
}

func newEvalsCommand(configPath *string, inherited func() (string, int, []string)) *cobra.Command {
	evals := &cobra.Command{Use: "evals", Short: "manage cached model evaluations"}
	var outputJSON bool
	update := &cobra.Command{Use: "update [REPOSITORY]", Args: cobra.MaximumNArgs(1), Short: "refresh evaluation results for installed models", RunE: func(cmd *cobra.Command, args []string) error {
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return err
		}
		index, err := modelmanager.BuildInstalledIndex(cfg.ModelsPath)
		if err != nil {
			return err
		}
		repos := modelmanager.InstalledRepositories(index)
		if len(args) == 1 {
			if !modelmanager.HasInstalledRepository(index, args[0]) {
				return fmt.Errorf("repository %q is not installed", args[0])
			}
			repos = []string{args[0]}
		}
		client, err := modelmanager.NewHFCLIClient(cfg.HuggingFaceToken)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "[evals] scanning %d installed repositor%s\n", len(repos), map[bool]string{true: "y", false: "ies"}[len(repos) == 1])
		summary := modelmanager.EvalUpdateSummary{SchemaVersion: 1, Scanned: len(repos), Results: make([]modelmanager.EvalUpdateResult, 0, len(repos))}
		for index, repo := range repos {
			fmt.Fprintf(cmd.ErrOrStderr(), "[evals] repository %d/%d: %s\n", index+1, len(repos), repo)
			result, reconcileErr := modelmanager.ReconcileEvalsWithProgress(cmd.Context(), client, cfg.ModelsPath, repo, func(message string) {
				fmt.Fprintf(cmd.ErrOrStderr(), "[evals]   %s\n", message)
			})
			if reconcileErr != nil {
				summary.Failed++
				summary.Errors = append(summary.Errors, modelmanager.EvalUpdateError{ModelRepositoryID: repo, Error: reconcileErr.Error()})
				fmt.Fprintf(cmd.ErrOrStderr(), "[evals]   failed: %s\n", reconcileErr)
				continue
			}
			fmt.Fprintf(cmd.ErrOrStderr(), "[evals]   complete: %s\n", result.Status)
			summary.Results = append(summary.Results, result)
			switch result.Status {
			case modelmanager.EvalStatusUnchanged:
				summary.Unchanged++
			case modelmanager.EvalStatusNotFound:
				summary.NotFound++
			case modelmanager.EvalStatusRemoved:
				summary.Removed++
			default:
				summary.Updated++
			}
		}
		if outputJSON {
			_ = json.NewEncoder(cmd.OutOrStdout()).Encode(summary)
		} else {
			for _, result := range summary.Results {
				authority := "NONE"
				if result.Authority != "" {
					authority = strings.ToUpper(string(result.Authority))
				}
				source := ""
				if result.SourceRepositoryID != "" {
					source = " source=" + result.SourceRepositoryID
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%-32s %-9s %s%s\n", result.ModelRepositoryID, authority, result.Status, source)
			}
			for _, failure := range summary.Errors {
				fmt.Fprintf(cmd.ErrOrStderr(), "%s ERROR %s\n", failure.ModelRepositoryID, failure.Error)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nScanned: %d  Updated: %d  Unchanged: %d  No eval: %d  Failed: %d\n", summary.Scanned, summary.Updated, summary.Unchanged, summary.NotFound, summary.Failed)
		}
		if summary.Failed > 0 {
			return fmt.Errorf("evaluation update failed for %d repository(s)", summary.Failed)
		}
		return nil
	}}
	update.Flags().BoolVar(&outputJSON, "json", false, "write JSON output")
	evals.AddCommand(update)
	return evals
}

func newUpdateCommand(configPath *string, inherited func() (string, int, []string)) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "update [MODEL]", Short: "update an installed model", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return err
		}
		client, err := modelmanager.NewHFCLIClient(cfg.HuggingFaceToken)
		if err != nil {
			return err
		}
		if !yes {
			initial := ""
			if len(args) > 0 {
				initial = args[0]
			}
			return modelmanager.RunInstalledInteractive(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), client, interactiveOptions(cfg, client), modelmanager.ActionUpdate, initial)
		}
		if len(args) != 1 {
			return fmt.Errorf("update --yes requires MODEL")
		}
		index, err := modelmanager.BuildInstalledIndex(cfg.ModelsPath)
		if err != nil {
			return err
		}
		installed, err := modelmanager.FindInstallation(index, args[0])
		if err != nil {
			return err
		}
		revision, files, err := client.ListFiles(cmd.Context(), installed.Manifest.RepositoryID)
		if err != nil {
			return err
		}
		var remote *modelmanager.ModelFile
		for i := range files {
			if files[i].Path == installed.Manifest.ModelFile {
				remote = &files[i]
				break
			}
		}
		if remote == nil {
			return fmt.Errorf("remote artifact no longer exists; select a replacement manually")
		}
		if revision == installed.Manifest.Revision {
			fmt.Fprintln(cmd.OutOrStdout(), "CURRENT")
			return nil
		}
		manifest, err := modelmanager.UpdateInstallation(cmd.Context(), client.Path, cfg.ModelsPath, installed, revision, *remote)
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(manifest)
	}}
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm update")
	return cmd
}

func newInstalledCommands(configPath *string, inherited func() (string, int, []string)) []*cobra.Command {
	load := func(cmd *cobra.Command) (induction.ModelManagerConfig, modelmanager.InstalledIndex, error) {
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return cfg, modelmanager.InstalledIndex{}, err
		}
		index, err := modelmanager.BuildInstalledIndex(cfg.ModelsPath)
		return cfg, index, err
	}
	runInteractive := func(cmd *cobra.Command, args []string, action modelmanager.InstalledAction) error {
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return err
		}
		client := &modelmanager.HFCLIClient{}
		if action == modelmanager.ActionUpdate {
			client, err = modelmanager.NewHFCLIClient(cfg.HuggingFaceToken)
			if err != nil {
				return err
			}
		}
		initial := ""
		if len(args) > 0 {
			initial = args[0]
		}
		return modelmanager.RunInstalledInteractive(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), client, interactiveOptions(cfg, client), action, initial)
	}
	var listJSON, installed, loaded bool
	listCmd := &cobra.Command{Use: "list", Short: "list installed or loaded models", RunE: func(cmd *cobra.Command, _ []string) error {
		if !installed && !loaded {
			return fmt.Errorf("list currently requires --installed or --loaded")
		}
		if installed && loaded {
			return fmt.Errorf("list accepts only one of --installed or --loaded")
		}
		if loaded {
			client, err := configuredClient(cmd, *configPath)
			if err != nil {
				return err
			}
			names, err := client.LoadedModelNames(cmd.Context())
			if err != nil {
				return err
			}
			if listJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(names)
			}
			if len(names) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No models currently loaded.")
				return nil
			}
			for _, name := range names {
				fmt.Fprintln(cmd.OutOrStdout(), name)
			}
			return nil
		}
		_, index, err := load(cmd)
		if err != nil {
			return err
		}
		for _, warning := range index.Warnings {
			fmt.Fprintln(cmd.ErrOrStderr(), "warning:", warning)
		}
		if listJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(index.Installations)
		}
		for _, item := range index.Installations {
			fmt.Fprintln(cmd.OutOrStdout(), item.Manifest.RepositoryID+"/"+item.Manifest.ModelFile)
		}
		return nil
	}}
	listCmd.Flags().BoolVar(&installed, "installed", false, "list installed models")
	listCmd.Flags().BoolVar(&loaded, "loaded", false, "list currently loaded models")
	listCmd.Flags().BoolVar(&listJSON, "json", false, "write JSON output")
	var detailsJSON bool
	detailsCmd := &cobra.Command{Use: "details [MODEL]", Short: "show installed model details", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !detailsJSON {
			return runInteractive(cmd, args, modelmanager.ActionDetails)
		}
		if len(args) != 1 {
			return fmt.Errorf("details --json requires MODEL")
		}
		_, index, err := load(cmd)
		if err != nil {
			return err
		}
		item, err := modelmanager.FindInstallation(index, args[0])
		if err != nil {
			return err
		}
		if detailsJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(item)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s\nrevision: %s\nartifact: %s\nmanifest: %s\n", args[0], item.Manifest.Revision, item.ArtifactPath, item.ManifestPath)
		return nil
	}}
	detailsCmd.Flags().BoolVar(&detailsJSON, "json", false, "write JSON output")
	var verifyJSON bool
	verifyCmd := &cobra.Command{Use: "verify [MODEL]", Short: "verify an installed model", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !verifyJSON {
			return runInteractive(cmd, args, modelmanager.ActionVerify)
		}
		if len(args) != 1 {
			return fmt.Errorf("verify --json requires MODEL")
		}
		_, index, err := load(cmd)
		if err != nil {
			return err
		}
		item, err := modelmanager.FindInstallation(index, args[0])
		if err != nil {
			return err
		}
		result, err := modelmanager.Verify(cmd.Context(), item)
		if err != nil {
			return err
		}
		if verifyJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		fmt.Fprintln(cmd.OutOrStdout(), result.Status)
		if result.Status != "VERIFIED" {
			return fmt.Errorf("verification status: %s", result.Status)
		}
		return nil
	}}
	verifyCmd.Flags().BoolVar(&verifyJSON, "json", false, "write JSON output")
	var removeYes bool
	removeCmd := &cobra.Command{Use: "remove [MODEL]", Short: "remove an installed model", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		if !removeYes {
			return runInteractive(cmd, args, modelmanager.ActionRemove)
		}
		if len(args) != 1 {
			return fmt.Errorf("remove --yes requires MODEL")
		}
		cfg, index, err := load(cmd)
		if err != nil {
			return err
		}
		item, err := modelmanager.FindInstallation(index, args[0])
		if err != nil {
			return err
		}
		if err := modelmanager.RemoveInstallation(cfg.ModelsPath, item); err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), args[0])
		return nil
	}}
	removeCmd.Flags().BoolVar(&removeYes, "yes", false, "confirm removal")
	return []*cobra.Command{listCmd, detailsCmd, verifyCmd, removeCmd}
}

func newDownloadCommand(configPath *string, inherited func() (string, int, []string)) *cobra.Command {
	var revision string
	var yes bool
	cmd := &cobra.Command{Use: "download REPOSITORY FILE", Short: "download a model file", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return fmt.Errorf("download requires --yes")
		}
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return err
		}
		client, err := modelmanager.NewHFCLIClient(cfg.HuggingFaceToken)
		if err != nil {
			return err
		}
		resolved, files, err := client.ListFiles(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		if revision == "" {
			revision = resolved
		}
		var selected *modelmanager.ModelFile
		for i := range files {
			if files[i].Path == args[1] {
				selected = &files[i]
				break
			}
		}
		if selected == nil {
			return fmt.Errorf("file %q does not exist in repository", args[1])
		}
		_, shards, shardErr := modelmanager.ShardSet(*selected, files)
		if shardErr != nil {
			return shardErr
		}
		if len(shards) > 1 {
			manifest, _, err := modelmanager.DownloadMulti(cmd.Context(), client.Path, cfg.ModelsPath, args[0], revision, shards, cfg.HuggingFaceToken)
			if err != nil {
				return err
			}
			if err := writeDownloadManifestAndEvals(cmd, manifest, client, cfg.ModelsPath); err != nil {
				return err
			}
			return nil
		}
		manifest, err := modelmanager.Download(cmd.Context(), client.Path, modelmanager.DownloadRequest{Repository: args[0], File: args[1], Revision: revision, ModelsPath: cfg.ModelsPath, Size: selected.Size, ETag: selected.ETag, LFSOID: selected.LFSOID, Overwrite: yes, Token: cfg.HuggingFaceToken})
		if err != nil {
			return err
		}
		if err := writeDownloadManifestAndEvals(cmd, manifest, client, cfg.ModelsPath); err != nil {
			return err
		}
		return nil
	}}
	cmd.Flags().StringVar(&revision, "revision", "", "immutable revision SHA")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm download")
	return cmd
}

func writeDownloadManifestAndEvals(cmd *cobra.Command, manifest modelmanager.Manifest, client *modelmanager.HFCLIClient, modelsPath string) error {
	if err := json.NewEncoder(cmd.OutOrStdout()).Encode(manifest); err != nil {
		return err
	}
	result, err := modelmanager.ReconcileEvals(cmd.Context(), client, modelsPath, manifest.RepositoryID)
	if err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "eval: %s error=%s\n", manifest.RepositoryID, err)
		return nil
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "eval: %s source=%s status=%s\n", manifest.RepositoryID, result.SourceRepositoryID, result.Status)
	return nil
}

func newFilesCommand(configPath *string, inherited func() (string, int, []string)) *cobra.Command {
	var outputJSON, revealAll bool
	cmd := &cobra.Command{Use: "files REPOSITORY", Short: "list files in a model repository", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return err
		}
		client, err := modelmanager.NewHFCLIClient(cfg.HuggingFaceToken)
		if err != nil {
			return err
		}
		revision, files, err := client.ListFiles(cmd.Context(), args[0])
		if err != nil {
			return err
		}
		files = modelmanager.FilterFiles(files, cfg.IncludePatterns, cfg.ExcludePatterns, cfg.PreferredQuantizations, revealAll)
		result := map[string]any{"schemaVersion": 1, "repositoryId": args[0], "revision": revision, "files": files}
		if outputJSON {
			return json.NewEncoder(cmd.OutOrStdout()).Encode(result)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Revision: %s\n", revision)
		for _, file := range files {
			fmt.Fprintf(cmd.OutOrStdout(), "%s\t%d\n", file.Path, file.Size)
		}
		return nil
	}}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "write JSON output")
	cmd.Flags().BoolVar(&revealAll, "all", false, "show all repository files")
	return cmd
}

func newSearchCommand(configPath *string, inherited func() (string, int, []string)) *cobra.Command {
	var outputJSON bool
	cmd := &cobra.Command{
		Use: "search [QUERY]", Args: cobra.MaximumNArgs(1),
		Short: "search model repositories",
		RunE: func(cmd *cobra.Command, args []string) error {
			modelsPath, searchResults, providers := inherited()
			parent := cmd.Parent()
			cfg, err := loadModelManagerConfig(*configPath, parent, modelsPath, searchResults, providers)
			if err != nil {
				return err
			}
			client, err := modelmanager.NewHFCLIClient(cfg.HuggingFaceToken)
			if err != nil {
				return err
			}
			query := ""
			if len(args) > 0 {
				query = args[0]
			}
			if !outputJSON {
				return modelmanager.RunInteractive(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), client, interactiveOptions(cfg, client), query)
			}
			results, err := modelmanager.SearchRanked(cmd.Context(), client, query, cfg.SearchResults, cfg.PreferredProviders)
			if err != nil {
				return err
			}
			if outputJSON {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(results)
			}
			for _, result := range results {
				fmt.Fprintln(cmd.OutOrStdout(), result.ID)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&outputJSON, "json", false, "write JSON output")
	return cmd
}

func interactiveOptions(cfg induction.ModelManagerConfig, client *modelmanager.HFCLIClient) modelmanager.InteractiveOptions {
	return modelmanager.InteractiveOptions{SearchResults: cfg.SearchResults, PreferredProviders: cfg.PreferredProviders, PreferredQuantizations: cfg.PreferredQuantizations, IncludePatterns: cfg.IncludePatterns, ExcludePatterns: cfg.ExcludePatterns, ModelsPath: cfg.ModelsPath, HFPath: client.Path, HuggingFaceToken: cfg.HuggingFaceToken}
}

func loadModelManagerConfig(path string, cmd *cobra.Command, modelsPath string, searchResults int, providers []string) (induction.ModelManagerConfig, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix("INDUCTION")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.SetDefault("ModelManager.SearchResults", 10)
	if err := v.ReadInConfig(); err != nil {
		return induction.ModelManagerConfig{}, fmt.Errorf("load config %q: %w", path, err)
	}
	manager := v.Sub("ModelManager")
	if manager == nil {
		manager = viper.New()
		manager.SetDefault("SearchResults", v.GetInt("ModelManager.SearchResults"))
		manager.SetDefault("ModelsPath", v.GetString("ModelManager.ModelsPath"))
	}
	var cfg induction.ModelManagerConfig
	if err := manager.UnmarshalExact(&cfg); err != nil {
		return induction.ModelManagerConfig{}, fmt.Errorf("decode config: %w", err)
	}
	// Read environment overrides explicitly so they retain precedence when a
	// subsection is decoded independently of the root Viper instance.
	if value := v.GetString("ModelManager.ModelsPath"); value != "" {
		cfg.ModelsPath = value
	}
	if value := v.GetInt("ModelManager.SearchResults"); value != 0 {
		cfg.SearchResults = value
	}
	if value := v.GetStringSlice("ModelManager.PreferredProviders"); len(value) > 0 {
		cfg.PreferredProviders = value
	}
	if cmd.Flags().Changed("models-path") {
		cfg.ModelsPath = modelsPath
	}
	if cmd.Flags().Changed("search-results") {
		cfg.SearchResults = searchResults
	}
	if cmd.Flags().Changed("preferred-provider") {
		cfg.PreferredProviders = providers
	}
	if err := cfg.NormalizeAndValidate(); err != nil {
		return induction.ModelManagerConfig{}, err
	}
	return cfg, nil
}

// Execute runs the root command and returns a process exit status.
func Execute() int {
	root := NewRootCommand()
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
