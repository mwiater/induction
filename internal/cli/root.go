// Package cli implements the induction command-line interface.
package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
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
	root := &cobra.Command{Use: "induction", SilenceUsage: true, SilenceErrors: true}
	root.PersistentFlags().StringVar(&configPath, "config", "induction.yaml", "configuration file")
	root.AddCommand(newModelManagerCommand(&configPath))
	root.AddCommand(newRuntimeCommand(&configPath))
	root.AddCommand(newInspectCommand(&configPath))
	root.AddCommand(newUICommand())
	return root
}

func newUICommand() *cobra.Command {
	ui := &cobra.Command{Use: "ui"}
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
		induction.WithLogger(log.New(cmd.ErrOrStderr(), "", 0))), nil
}

func newInspectCommand(configPath *string) *cobra.Command {
	server := &cobra.Command{Use: "server", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
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
	server.Flags().Bool("json", false, "write JSON output")
	model := &cobra.Command{Use: "model MODEL", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	model.Flags().Bool("json", false, "write JSON output")
	root := &cobra.Command{Use: "inspect"}
	root.AddCommand(server, model)
	return root
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
	status := &cobra.Command{Use: "status", Args: cobra.NoArgs, RunE: func(cmd *cobra.Command, _ []string) error {
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
		cmd := &cobra.Command{Use: name + " MODEL", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	root := &cobra.Command{Use: "runtime"}
	root.AddCommand(status, operation("load", func(c *induction.Client, ctx context.Context, model string) (*induction.RuntimeOperation, error) {
		return c.LoadModel(ctx, model)
	}), operation("unload", func(c *induction.Client, ctx context.Context, model string) (*induction.RuntimeOperation, error) {
		return c.UnloadModel(ctx, model)
	}))
	var switchJSON bool
	switchCmd := &cobra.Command{Use: "switch MODEL", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
		Use:     "model-manager [initial-query]",
		Aliases: []string{"models"},
		Args:    cobra.MaximumNArgs(1),
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			return modelmanager.LogInteraction("command_started", "command="+cmd.CommandPath(), "args="+strings.Join(args, " "))
		},
		PersistentPostRun: func(cmd *cobra.Command, _ []string) {
			_ = modelmanager.LogInteraction("command_completed", "command="+cmd.CommandPath())
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadModelManagerConfig(*configPath, cmd, modelsPath, searchResults, providers)
			if err != nil {
				return err
			}
			client, err := modelmanager.NewHFCLIClient()
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
	return cmd
}

func newUpdateCommand(configPath *string, inherited func() (string, int, []string)) *cobra.Command {
	var yes bool
	cmd := &cobra.Command{Use: "update [MODEL]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return err
		}
		client, err := modelmanager.NewHFCLIClient()
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
			client, err = modelmanager.NewHFCLIClient()
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
	var listJSON, installed bool
	listCmd := &cobra.Command{Use: "list", RunE: func(cmd *cobra.Command, _ []string) error {
		if !installed {
			return fmt.Errorf("list currently requires --installed")
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
	listCmd.Flags().BoolVar(&listJSON, "json", false, "write JSON output")
	var detailsJSON bool
	detailsCmd := &cobra.Command{Use: "details [MODEL]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	verifyCmd := &cobra.Command{Use: "verify [MODEL]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	removeCmd := &cobra.Command{Use: "remove [MODEL]", Args: cobra.MaximumNArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
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
	cmd := &cobra.Command{Use: "download REPOSITORY FILE", Args: cobra.ExactArgs(2), RunE: func(cmd *cobra.Command, args []string) error {
		if !yes {
			return fmt.Errorf("download requires --yes")
		}
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return err
		}
		client, err := modelmanager.NewHFCLIClient()
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
			manifest, _, err := modelmanager.DownloadMulti(cmd.Context(), client.Path, cfg.ModelsPath, args[0], revision, shards)
			if err != nil {
				return err
			}
			return json.NewEncoder(cmd.OutOrStdout()).Encode(manifest)
		}
		manifest, err := modelmanager.Download(cmd.Context(), client.Path, modelmanager.DownloadRequest{Repository: args[0], File: args[1], Revision: revision, ModelsPath: cfg.ModelsPath, Size: selected.Size, ETag: selected.ETag, LFSOID: selected.LFSOID, Overwrite: yes})
		if err != nil {
			return err
		}
		return json.NewEncoder(cmd.OutOrStdout()).Encode(manifest)
	}}
	cmd.Flags().StringVar(&revision, "revision", "", "immutable revision SHA")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm download")
	return cmd
}

func newFilesCommand(configPath *string, inherited func() (string, int, []string)) *cobra.Command {
	var outputJSON, revealAll bool
	cmd := &cobra.Command{Use: "files REPOSITORY", Args: cobra.ExactArgs(1), RunE: func(cmd *cobra.Command, args []string) error {
		modelsPath, searchResults, providers := inherited()
		cfg, err := loadModelManagerConfig(*configPath, cmd.Parent(), modelsPath, searchResults, providers)
		if err != nil {
			return err
		}
		client, err := modelmanager.NewHFCLIClient()
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
		RunE: func(cmd *cobra.Command, args []string) error {
			modelsPath, searchResults, providers := inherited()
			parent := cmd.Parent()
			cfg, err := loadModelManagerConfig(*configPath, parent, modelsPath, searchResults, providers)
			if err != nil {
				return err
			}
			client, err := modelmanager.NewHFCLIClient()
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
	return modelmanager.InteractiveOptions{SearchResults: cfg.SearchResults, PreferredProviders: cfg.PreferredProviders, PreferredQuantizations: cfg.PreferredQuantizations, IncludePatterns: cfg.IncludePatterns, ExcludePatterns: cfg.ExcludePatterns, ModelsPath: cfg.ModelsPath, HFPath: client.Path}
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
	target, _, _ := root.Find(os.Args[1:])
	if err := root.Execute(); err != nil {
		if target != nil && strings.Contains(target.CommandPath(), "model-manager") {
			_ = modelmanager.LogInteraction("command_failed", "command="+target.CommandPath(), "error="+err.Error())
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
