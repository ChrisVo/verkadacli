package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// These can be set at build time via -ldflags "-X verkcli/internal/cli.version=... -X verkcli/internal/cli.commit=..."
var (
	version = "dev"
	commit  = "none"
)

func NewVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "%s (commit %s)\n", version, commit)
			return nil
		},
	}
}
