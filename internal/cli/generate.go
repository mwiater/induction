package cli

import (
	"fmt"

	induction "github.com/mwiater/induction"
	"github.com/spf13/cobra"
)

func newGenerateCommand() *cobra.Command {
	generate := &cobra.Command{Use: "generate", Short: "generate derived Induction data artifacts"}
	generateDashboard := &cobra.Command{
		Use:   "dashboard",
		Short: "generate dashboard metrics from persisted sessions",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			metrics, err := induction.GenerateDashboardMetrics(induction.DashboardGenerateOptions{})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Dashboard generated: %s\nMetrics: %s\nSessions: %d\nSnapshots: %d\nModels: %d\n", induction.DefaultDashboardHTMLPath, induction.DefaultDashboardMetricsPath, metrics.Source.SessionsLoaded, metrics.Source.SnapshotsIncluded, metrics.Source.Models)
			if metrics.Source.SnapshotsSkipped > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Skipped snapshots: %d\n", metrics.Source.SnapshotsSkipped)
			}
			return nil
		},
	}
	generate.AddCommand(generateDashboard)
	return generate
}
