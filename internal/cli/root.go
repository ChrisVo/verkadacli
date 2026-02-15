package cli

import (
	"os"

	"github.com/spf13/cobra"
)

type rootFlags struct {
	ConfigPath string
	Profile    string
	BaseURL    string
	OrgID      string
	APIKey     string
	Token      string
	Debug      bool
	Output     string
	Headers    []string
}

// NewRootCmd builds the root command and wires subcommands.
func NewRootCmd() *cobra.Command {
	var rf rootFlags

	cmd := &cobra.Command{
		Use:           "verkcli",
		Short:         "CLI for Verkada APIs",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().StringVar(&rf.ConfigPath, "config", "", "Config file path (default: $XDG_CONFIG_HOME/verkcli/config.json)")
	cmd.PersistentFlags().StringVar(&rf.Profile, "profile", "", "Config profile to use (or set VERKCLI_PROFILE / VERKADA_PROFILE)")
	cmd.PersistentFlags().StringVar(&rf.BaseURL, "base-url", "", "Base URL (or set VERKCLI_BASE_URL / VERKADA_BASE_URL)")
	cmd.PersistentFlags().StringVar(&rf.OrgID, "org-id", "", "Organization ID (or set VERKCLI_ORG_ID / VERKADA_ORG_ID)")
	cmd.PersistentFlags().StringVar(&rf.APIKey, "api-key", "", "API key (or set VERKCLI_API_KEY / VERKADA_API_KEY)")
	cmd.PersistentFlags().StringVar(&rf.Token, "token", "", "Bearer token (or set VERKCLI_TOKEN / VERKADA_TOKEN)")
	cmd.PersistentFlags().StringVar(&rf.Output, "output", "text", "Output format: text|json")
	cmd.PersistentFlags().BoolVar(&rf.Debug, "debug", false, "Enable debug logging")
	cmd.PersistentFlags().StringArrayVarP(&rf.Headers, "header", "H", nil, "Extra header (repeatable), e.g. -H 'X-Foo: bar'")

	_ = cmd.PersistentFlags().MarkHidden("token") // keep surface area small; headers cover most auth modes

	cmd.SetOut(os.Stdout)
	cmd.SetErr(os.Stderr)

	cmd.AddCommand(NewVersionCmd())
	cmd.AddCommand(NewConfigCmd(&rf))
	cmd.AddCommand(NewProfilesCmd(&rf))
	cmd.AddCommand(NewLoginCmd(&rf))
	cmd.AddCommand(NewRequestCmd(&rf))
	cmd.AddCommand(NewCamerasCmd(&rf))

	return cmd
}
