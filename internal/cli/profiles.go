package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// NewProfilesCmd provides ergonomic top-level profile management commands.
//
// This intentionally overlaps with `verkcli config profiles ...` as a convenience.
func NewProfilesCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "Manage named profiles",
	}

	cmd.AddCommand(newProfilesListCmd(rf))
	cmd.AddCommand(newProfilesAddCmd(rf))
	cmd.AddCommand(newProfilesPathCmd(rf))
	return cmd
}

func newProfilesListCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List profiles in the config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfilesList(cmd, rf)
		},
	}
	return cmd
}

func newProfilesPathCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "path",
		Short: "Print the config file path where profiles are stored",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolveConfigPath(rf.ConfigPath)
			if err != nil {
				return err
			}

			exists := true
			if _, err := os.Stat(p); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					exists = false
				} else {
					return err
				}
			}

			if rf.Output == "json" {
				blob, err := json.MarshalIndent(map[string]any{
					"config_path": p,
					"exists":      exists,
				}, "", "  ")
				if err != nil {
					return err
				}
				blob = append(blob, '\n')
				_, _ = cmd.OutOrStdout().Write(blob)
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), p)
			if !exists {
				fmt.Fprintln(cmd.ErrOrStderr(), "config file does not exist yet (run: verkcli login or verkcli config init)")
			}
			return nil
		},
	}
	return cmd
}

func newProfilesAddCmd(rf *rootFlags) *cobra.Command {
	var noPrompt bool
	var noVerify bool
	var verifyTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "add [PROFILE]",
		Short: "Add (or update) a profile by running the login flow",
		Args:  cobra.MaximumNArgs(1),
		Example: strings.TrimSpace(`
  verkcli profiles add work
  verkcli profiles add   # prompt for profile name, then credentials
  verkcli profiles add eu --base-url https://api.eu.verkada.com
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := ""
			if len(args) == 1 {
				name = strings.TrimSpace(args[0])
			}

			if name == "" && !noPrompt {
				s, err := promptString(cmd, "Profile", firstNonEmpty(rf.Profile, envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE"), "default"), false /* secret */)
				if err != nil {
					return err
				}
				name = strings.TrimSpace(s)
			}

			if name == "" {
				return errors.New("profile is empty")
			}
			if strings.ContainsAny(name, " \t") {
				return fmt.Errorf("profile name must not contain spaces")
			}

			// Force the login flow to use the provided profile name (and skip prompting for it).
			prev := rf.Profile
			rf.Profile = name
			defer func() {
				rf.Profile = prev
			}()

			return runLogin(cmd, rf, noPrompt, noVerify, verifyTimeout)
		},
	}

	cmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "Fail instead of prompting for missing values")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "Skip preflight verification against the Verkada API")
	cmd.Flags().DurationVar(&verifyTimeout, "verify-timeout", 20*time.Second, "Timeout for login preflight verification")
	return cmd
}
