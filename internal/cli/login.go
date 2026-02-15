package cli

import (
	"bufio"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func NewLoginCmd(rf *rootFlags) *cobra.Command {
	var noPrompt bool
	var noVerify bool
	var verifyTimeout time.Duration

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Save base URL and API key to local config",
		Long: strings.TrimSpace(`
Login writes credentials into your local config file so subsequent commands can authenticate.

Examples:
  verkcli login --base-url https://api.verkada.com --api-key $VERKCLI_API_KEY
  verkcli --profile eu login --base-url https://api.eu.verkada.com --api-key $VERKCLI_API_KEY
  verkcli login   # prompts and saves to config
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolveConfigPath(rf.ConfigPath)
			if err != nil {
				return err
			}

			// Start from existing config if present; otherwise start from empty.
			cf, err := loadConfig(p)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					cf = ConfigFile{Profiles: map[string]Config{}}
				} else {
					return err
				}
			}
			normalizeConfigFile(&cf)

			profileName := firstNonEmpty(rf.Profile, envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE"), cf.CurrentProfile, "default")
			if !noPrompt && rf.Profile == "" && envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE") == "" {
				for {
					s, err := promptString(cmd, "Profile", profileName, false /* secret */)
					if err != nil {
						return err
					}
					s = strings.TrimSpace(s)
					if s == "" {
						fmt.Fprintln(cmd.ErrOrStderr(), "Profile is empty.")
						continue
					}
					if strings.ContainsAny(s, " \t") {
						fmt.Fprintln(cmd.ErrOrStderr(), "Profile name must not contain spaces.")
						continue
					}
					profileName = s
					break
				}
			}

			profile := cf.Profiles[profileName] // ok if missing; zero value is fine
			if profile.Headers == nil {
				profile.Headers = map[string]string{}
			}

			baseURL := firstNonEmpty(rf.BaseURL, envFirst("", "VERKCLI_BASE_URL", "VERKADA_BASE_URL"), profile.BaseURL, "https://api.verkada.com")
			// Don't suggest Command web UI URLs as the interactive default, but don't override explicit values.
			baseURLPromptDefault := sanitizeBaseURLDefault(baseURL)
			orgID := firstNonEmpty(rf.OrgID, envFirst("", "VERKCLI_ORG_ID", "VERKADA_ORG_ID"), profile.OrgID)
			apiKey := firstNonEmpty(rf.APIKey, envFirst("", "VERKCLI_API_KEY", "VERKADA_API_KEY"), profile.Auth.APIKey)
			token := firstNonEmpty(rf.Token, envFirst("", "VERKCLI_TOKEN", "VERKADA_TOKEN"), profile.Auth.Token)

			if !noPrompt {
				// Keep prompting until base URL validates, so users don't get stuck on a single bad paste.
				for {
					s, err := promptString(cmd, "Base URL", baseURLPromptDefault, false /* secret */)
					if err != nil {
						return err
					}
					s = strings.TrimSpace(s)
					if strings.ContainsAny(s, " \t") {
						// Common mistake: pasting flags into the prompt.
						fmt.Fprintln(cmd.ErrOrStderr(), "Base URL should be a single URL. Don't paste flags here. Example: verkcli login --base-url https://api.verkada.com --api-key ...")
						continue
					}
					if s == "" {
						fmt.Fprintln(cmd.ErrOrStderr(), "Base URL is empty.")
						continue
					}
					if _, err := validateBaseURL(s); err != nil {
						fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
						continue
					}
					baseURL = s
					break
				}

				// Only prompt for API key if not already set via flags/env/config.
				if strings.TrimSpace(apiKey) == "" {
					for {
						s, err := promptString(cmd, "API key", "", true /* secret */)
						if err != nil {
							return err
						}
						s = strings.TrimSpace(s)
						if strings.ContainsAny(s, " \t") {
							fmt.Fprintln(cmd.ErrOrStderr(), "API key should not contain spaces. If you're trying to pass flags, run: verkcli login --base-url ... --api-key ...")
							continue
						}
						if s == "" {
							fmt.Fprintln(cmd.ErrOrStderr(), "API key is empty.")
							continue
						}
						apiKey = s
						break
					}
				}
			}

			baseURL = strings.TrimSpace(baseURL)
			apiKey = strings.TrimSpace(apiKey)

			if baseURL == "" {
				return errors.New("base URL is empty (set --base-url or VERKCLI_BASE_URL / VERKADA_BASE_URL)")
			}
			if _, err := validateBaseURL(baseURL); err != nil {
				return err
			}
			if apiKey == "" {
				return errors.New("API key is empty (set --api-key or VERKCLI_API_KEY / VERKADA_API_KEY)")
			}

			// If org_id is still empty, best-effort auto-discover it. This helps unblock
			// footage streaming commands without making org_id mandatory for basic camera APIs.
			if strings.TrimSpace(orgID) == "" {
				client := &http.Client{Timeout: 15 * time.Second}
				tmpCfg := profile
				tmpCfg.BaseURL = baseURL
				tmpCfg.Auth.APIKey = apiKey
				tmpCfg.Auth.Token = token
				filled, err := ensureOrgID(client, &tmpCfg, rf)
				if err != nil && rf.Debug {
					fmt.Fprintln(cmd.ErrOrStderr(), err.Error())
				}
				if filled {
					orgID = tmpCfg.OrgID
				} else if !noPrompt {
					// Fall back to asking only if discovery didn't work.
					s, err := promptString(cmd, "Org ID (required for footage streaming)", orgID, false /* secret */)
					if err != nil {
						return err
					}
					s = strings.TrimSpace(s)
					if strings.ContainsAny(s, " \t") {
						fmt.Fprintln(cmd.ErrOrStderr(), "Org ID should not contain spaces. If you're trying to pass flags, run: verkcli login --org-id ...")
					} else {
						orgID = s
					}
				}
			}

			// Verify the provided (or discovered) config works before persisting it.
			if !noVerify {
				client := &http.Client{Timeout: verifyTimeout}
				tmpCfg := profile
				tmpCfg.BaseURL = baseURL
				tmpCfg.OrgID = orgID
				tmpCfg.Auth.APIKey = apiKey
				tmpCfg.Auth.Token = token
				if err := verifyLoginPreflight(client, &tmpCfg, rf); err != nil {
					return err
				}
				// Carry any discovered values (e.g., token refresh) into the persisted profile.
				if strings.TrimSpace(tmpCfg.OrgID) != "" {
					orgID = strings.TrimSpace(tmpCfg.OrgID)
				}
				if strings.TrimSpace(tmpCfg.Auth.Token) != "" {
					token = strings.TrimSpace(tmpCfg.Auth.Token)
				}
				if tmpCfg.Auth.TokenAcquiredAt != 0 {
					profile.Auth.TokenAcquiredAt = tmpCfg.Auth.TokenAcquiredAt
				}
			}

			profile.BaseURL = baseURL
			profile.Auth.APIKey = apiKey
			// Keep org ID if present (used for footage streaming endpoints).
			if strings.TrimSpace(orgID) != "" {
				profile.OrgID = strings.TrimSpace(orgID)
			}
			// Keep token if present; it's hidden at the root flags but still supported.
			if strings.TrimSpace(token) != "" {
				profile.Auth.Token = token
			}

			cf.Profiles[profileName] = profile
			cf.CurrentProfile = profileName

			if err := writeConfig(p, cf); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", p)
			return nil
		},
	}

	cmd.Flags().BoolVar(&noPrompt, "no-prompt", false, "Fail instead of prompting for missing values")
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "Skip preflight verification against the Verkada API")
	cmd.Flags().DurationVar(&verifyTimeout, "verify-timeout", 20*time.Second, "Timeout for login preflight verification")
	return cmd
}

func sanitizeBaseURLDefault(s string) string {
	u, err := url.Parse(strings.TrimSpace(s))
	if err != nil {
		return s
	}
	host := strings.ToLower(u.Host)
	if strings.HasSuffix(host, ".command.verkada.com") || host == "command.verkada.com" {
		return "https://api.verkada.com"
	}
	return s
}

func validateBaseURL(s string) (*url.URL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL %q: %w", s, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("invalid base URL %q: scheme must be http or https", s)
	}
	if u.Host == "" {
		return nil, fmt.Errorf("invalid base URL %q: host is empty", s)
	}
	// Common pitfall: Command web UI host, not the API host.
	if strings.HasSuffix(strings.ToLower(u.Host), ".command.verkada.com") || strings.EqualFold(u.Host, "command.verkada.com") {
		return nil, fmt.Errorf("invalid base URL %q: this looks like the Command web UI. Use https://api.verkada.com (or https://api.eu.verkada.com / https://api.au.verkada.com)", s)
	}
	return u, nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func promptString(cmd *cobra.Command, label, def string, secret bool) (string, error) {
	out := cmd.ErrOrStderr() // prompts go to stderr
	if def != "" {
		fmt.Fprintf(out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(out, "%s: ", label)
	}

	// Best-effort hidden input for secrets when reading from a real TTY.
	if secret && cmd.InOrStdin() == os.Stdin && term.IsTerminal(int(os.Stdin.Fd())) {
		b, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(out) // newline after password input
		if err != nil {
			return "", err
		}
		s := strings.TrimSpace(string(b))
		if s == "" {
			return def, nil
		}
		return s, nil
	}

	r := bufio.NewReader(cmd.InOrStdin())
	line, err := r.ReadString('\n')
	if err != nil && !errors.Is(err, os.ErrClosed) {
		// If stdin has no newline, ReadString can return data with err==io.EOF; keep the data.
	}
	line = strings.TrimSpace(line)
	if line == "" {
		return def, nil
	}
	return line, nil
}
