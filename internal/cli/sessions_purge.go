package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var purgeDataDirectories = []string{".sessions", ".pipeline-artifacts", ".batches"}

func newPurgeSessionsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "purge",
		Short: "delete all local sessions, pipeline artifacts, and batches",
		Long:  "Delete .sessions/, .pipeline-artifacts/, and .batches/ from the current working directory. This permanently removes their contents.",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := purgeLocalData("."); err != nil {
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Purged .sessions/, .pipeline-artifacts/, and .batches/ from the current working directory.")
			return nil
		},
	}
}

func purgeLocalData(root string) error {
	// Check all three names before removing anything. Refuse symlinks and
	// non-directories so a surprising entry cannot redirect or widen deletion.
	for _, name := range purgeDataDirectories {
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect %s: %w", path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("refusing to purge %s: expected a directory, found a file or symlink", path)
		}
	}
	for _, name := range purgeDataDirectories {
		path := filepath.Join(root, name)
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("purge %s: %w", path, err)
		}
	}
	return nil
}
