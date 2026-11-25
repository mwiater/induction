package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	induction "github.com/mwiater/induction"
	"github.com/spf13/cobra"
)

const v1ModelsPath = "data/.local/v1_models.json"

// newModelsRefreshCommand creates the function used by this package.
func newModelsRefreshCommand(configPath *string) *cobra.Command {
	return &cobra.Command{
		Use:   "refresh",
		Short: "refresh the local raw /v1/models catalog",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := induction.LoadConfig(*configPath)
			if err != nil {
				return err
			}
			if err := refreshV1Models(cmd.Context(), cfg.Server, v1ModelsPath, nil); err != nil {
				return err
			}
			_, err = fmt.Fprintf(cmd.OutOrStdout(), "Refreshed %s from %s/v1/models.\n", v1ModelsPath, strings.TrimRight(cfg.Server, "/"))
			return err
		},
	}
}

func refreshV1Models(ctx context.Context, endpoint, destination string, client *http.Client) error {
	if client == nil {
		client = http.DefaultClient
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(endpoint, "/")+"/v1/models", http.NoBody)
	if err != nil {
		return fmt.Errorf("create models request: %w", err)
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch /v1/models: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("fetch /v1/models: unexpected status %s", response.Status)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read /v1/models response: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return fmt.Errorf("create models catalog directory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".v1_models.json-*")
	if err != nil {
		return fmt.Errorf("create models catalog temporary file: %w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("set models catalog permissions: %w", err)
	}
	if _, err := temporary.Write(body); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write models catalog: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close models catalog temporary file: %w", err)
	}
	if err := os.Rename(temporaryName, destination); err != nil {
		return fmt.Errorf("replace models catalog: %w", err)
	}
	return nil
}
