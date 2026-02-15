package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type Config struct {
	BaseURL string            `json:"base_url"`
	OrgID   string            `json:"org_id,omitempty"`
	Auth    AuthConfig        `json:"auth,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Labels  *LocalLabels      `json:"labels,omitempty"`
}

type AuthConfig struct {
	APIKey          string `json:"api_key,omitempty"`
	Token           string `json:"token,omitempty"`             // x-verkada-auth
	TokenAcquiredAt int64  `json:"token_acquired_at,omitempty"` // unix seconds
}

type LocalLabels struct {
	Cameras map[string]string `json:"cameras,omitempty"`
}

// ConfigFile is the on-disk config format. It supports named profiles.
//
// Backward compatibility: legacy configs may contain top-level base_url/auth/headers,
// which are treated as an implicit "default" profile on load.
type ConfigFile struct {
	CurrentProfile string            `json:"current_profile,omitempty"`
	Profiles       map[string]Config `json:"profiles,omitempty"`

	// Legacy fields (pre-profiles).
	BaseURL string            `json:"base_url,omitempty"`
	Auth    *AuthConfig       `json:"auth,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
}

func defaultConfigPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	// Default to a non-trademarked config dir, but fall back to legacy configs
	// if they already exist on disk.
	newPath := filepath.Join(dir, "verkcli", "config.json")
	legacyPath := filepath.Join(dir, "verkada", "config.json")
	if _, err := os.Stat(newPath); err == nil {
		return newPath, nil
	}
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath, nil
	}
	return newPath, nil
}

func loadConfig(path string) (ConfigFile, error) {
	var cfg ConfigFile
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	normalizeConfigFile(&cfg)
	return cfg, nil
}

func writeConfig(path string, cfg ConfigFile) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	normalizeConfigFile(&cfg)
	// Write profiles format only.
	cfg.BaseURL = ""
	cfg.Auth = nil
	cfg.Headers = nil

	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0o600)
}

func normalizeConfigFile(cfg *ConfigFile) {
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Config{}
	}

	// If legacy fields are present, materialize them as the "default" profile.
	legacyHasData := strings.TrimSpace(cfg.BaseURL) != "" ||
		(cfg.Auth != nil && (strings.TrimSpace(cfg.Auth.APIKey) != "" || strings.TrimSpace(cfg.Auth.Token) != "")) ||
		cfg.Headers != nil
	if legacyHasData {
		if _, ok := cfg.Profiles["default"]; !ok {
			h := cfg.Headers
			if h == nil {
				h = map[string]string{}
			}
			var a AuthConfig
			if cfg.Auth != nil {
				a = *cfg.Auth
			}
			cfg.Profiles["default"] = Config{
				BaseURL: cfg.BaseURL,
				Auth:    a,
				Headers: h,
			}
		}
		if strings.TrimSpace(cfg.CurrentProfile) == "" {
			cfg.CurrentProfile = "default"
		}
	}

	// Ensure profile headers maps are non-nil for ergonomics elsewhere.
	for k, p := range cfg.Profiles {
		if p.Headers == nil {
			p.Headers = map[string]string{}
		}
		if p.Labels == nil {
			p.Labels = &LocalLabels{Cameras: map[string]string{}}
		} else if p.Labels.Cameras == nil {
			p.Labels.Cameras = map[string]string{}
		}
		cfg.Profiles[k] = p
	}

	if strings.TrimSpace(cfg.CurrentProfile) == "" && len(cfg.Profiles) > 0 {
		// Prefer default if present, otherwise pick any stable entry.
		if _, ok := cfg.Profiles["default"]; ok {
			cfg.CurrentProfile = "default"
		} else {
			for name := range cfg.Profiles {
				cfg.CurrentProfile = name
				break
			}
		}
	}
}

func resolveConfigPath(flagPath string) (string, error) {
	if flagPath != "" {
		return flagPath, nil
	}
	return defaultConfigPath()
}

func NewConfigCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage local CLI config",
	}

	cmd.AddCommand(newConfigInitCmd(rf))
	cmd.AddCommand(newConfigViewCmd(rf))
	cmd.AddCommand(newConfigUseCmd(rf))
	cmd.AddCommand(newConfigProfilesCmd(rf))

	return cmd
}

func newConfigUseCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "use PROFILE",
		Short: "Set the default profile in the config file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolveConfigPath(rf.ConfigPath)
			if err != nil {
				return err
			}
			cf, err := loadConfig(p)
			if err != nil {
				return err
			}
			name := strings.TrimSpace(args[0])
			if name == "" {
				return errors.New("profile name is empty")
			}
			if _, ok := cf.Profiles[name]; !ok {
				return fmt.Errorf("profile %q not found in %s", name, p)
			}
			cf.CurrentProfile = name
			if err := writeConfig(p, cf); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "current profile: %s\n", name)
			return nil
		},
	}
	return cmd
}

func newConfigProfilesCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profiles",
		Short: "Manage named profiles",
	}
	cmd.AddCommand(newConfigProfilesListCmd(rf))
	return cmd
}

func newConfigProfilesListCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List profiles in the config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfilesList(cmd, rf)
		},
	}
	return cmd
}

func runProfilesList(cmd *cobra.Command, rf *rootFlags) error {
	p, err := resolveConfigPath(rf.ConfigPath)
	if err != nil {
		return err
	}
	cf, err := loadConfig(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Empty config: just list nothing (or empty JSON).
			cf = ConfigFile{Profiles: map[string]Config{}}
		} else {
			return err
		}
	}
	normalizeConfigFile(&cf)

	names := make([]string, 0, len(cf.Profiles))
	for n := range cf.Profiles {
		names = append(names, n)
	}
	sort.Strings(names)

	out := cmd.OutOrStdout()
	if rf.Output == "json" {
		type profileView struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
		}
		profiles := make([]profileView, 0, len(names))
		for _, n := range names {
			profiles = append(profiles, profileView{Name: n, Current: n == cf.CurrentProfile})
		}
		blob, err := json.MarshalIndent(map[string]any{
			"current_profile": cf.CurrentProfile,
			"profiles":        profiles,
		}, "", "  ")
		if err != nil {
			return err
		}
		blob = append(blob, '\n')
		_, _ = out.Write(blob)
		return nil
	}

	for _, n := range names {
		marker := " "
		if n == cf.CurrentProfile {
			marker = "*"
		}
		fmt.Fprintf(out, "%s %s\n", marker, n)
	}
	return nil
}

func newConfigInitCmd(rf *rootFlags) *cobra.Command {
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create a default config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			p, err := resolveConfigPath(rf.ConfigPath)
			if err != nil {
				return err
			}
			if !force {
				if _, err := os.Stat(p); err == nil {
					return fmt.Errorf("config already exists at %s (use --force to overwrite)", p)
				}
			}

			profile := Config{
				BaseURL: envFirst("", "VERKCLI_BASE_URL", "VERKADA_BASE_URL"),
				OrgID:   envFirst("", "VERKCLI_ORG_ID", "VERKADA_ORG_ID"),
				Auth: AuthConfig{
					APIKey: envFirst("", "VERKCLI_API_KEY", "VERKADA_API_KEY"),
					Token:  envFirst("", "VERKCLI_TOKEN", "VERKADA_TOKEN"),
				},
				Headers: map[string]string{},
			}
			if profile.BaseURL == "" {
				profile.BaseURL = "https://api.verkada.com"
			}

			cfg := ConfigFile{
				CurrentProfile: "default",
				Profiles: map[string]Config{
					"default": profile,
				},
			}
			if err := writeConfig(p, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", p)
			return nil
		},
	}

	cmd.Flags().BoolVar(&force, "force", false, "Overwrite if config already exists")
	return cmd
}

func newConfigViewCmd(rf *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "view",
		Short: "Print the effective config (file + env + flags)",
		RunE: func(cmd *cobra.Command, args []string) error {
			profileName, ecfg, err := effectiveProfileConfig(*rf)
			if err != nil {
				if errors.Is(err, os.ErrNotExist) {
					// Still allow viewing env/flags-only config.
					profileName = firstNonEmpty(rf.Profile, envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE"), "default")
					ecfg = Config{
						BaseURL: envFirst("", "VERKCLI_BASE_URL", "VERKADA_BASE_URL"),
						OrgID:   envFirst("", "VERKCLI_ORG_ID", "VERKADA_ORG_ID"),
						Auth: AuthConfig{
							APIKey: envFirst("", "VERKCLI_API_KEY", "VERKADA_API_KEY"),
							Token:  envFirst("", "VERKCLI_TOKEN", "VERKADA_TOKEN"),
						},
						Headers: map[string]string{},
					}
				} else {
					return err
				}
			}

			view := struct {
				Profile string `json:"profile"`
				Config
			}{
				Profile: profileName,
				Config:  ecfg,
			}
			b, err := json.MarshalIndent(view, "", "  ")
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return nil
		},
	}
	return cmd
}

func effectiveConfig(rf rootFlags) (Config, error) {
	_, cfg, err := effectiveProfileConfig(rf)
	return cfg, err
}

func effectiveProfileConfig(rf rootFlags) (string, Config, error) {
	p, err := resolveConfigPath(rf.ConfigPath)
	if err != nil {
		return "", Config{}, err
	}

	cf, err := loadConfig(p)
	if err != nil {
		return "", Config{}, err
	}

	profileName := firstNonEmpty(rf.Profile, envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE"), cf.CurrentProfile, "default")
	profile, ok := cf.Profiles[profileName]
	if !ok {
		return "", Config{}, fmt.Errorf("profile %q not found in %s", profileName, p)
	}

	// Env overrides config.
	if v := envFirst("", "VERKCLI_BASE_URL", "VERKADA_BASE_URL"); v != "" {
		profile.BaseURL = v
	}
	if v := envFirst("", "VERKCLI_ORG_ID", "VERKADA_ORG_ID"); v != "" {
		profile.OrgID = v
	}
	if v := envFirst("", "VERKCLI_API_KEY", "VERKADA_API_KEY"); v != "" {
		profile.Auth.APIKey = v
	}
	if v := envFirst("", "VERKCLI_TOKEN", "VERKADA_TOKEN"); v != "" {
		profile.Auth.Token = v
	}

	// Flags override env/config.
	if rf.BaseURL != "" {
		profile.BaseURL = rf.BaseURL
	}
	if rf.OrgID != "" {
		profile.OrgID = rf.OrgID
	}
	if rf.APIKey != "" {
		profile.Auth.APIKey = rf.APIKey
	}
	if rf.Token != "" {
		profile.Auth.Token = rf.Token
	}

	if profile.BaseURL == "" {
		return "", Config{}, errors.New("base URL is empty (set in config, VERKCLI_BASE_URL / VERKADA_BASE_URL, or --base-url)")
	}
	if profile.Headers == nil {
		profile.Headers = map[string]string{}
	}
	return profileName, profile, nil
}

func envFirst(def string, keys ...string) string {
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok {
			return v
		}
	}
	return def
}

func persistProfileOrgID(rf rootFlags, orgID string) error {
	orgID = strings.TrimSpace(orgID)
	if orgID == "" {
		return nil
	}
	p, err := resolveConfigPath(rf.ConfigPath)
	if err != nil {
		return err
	}
	cf, err := loadConfig(p)
	if err != nil {
		return err
	}
	normalizeConfigFile(&cf)
	profileName := firstNonEmpty(rf.Profile, envFirst("", "VERKCLI_PROFILE", "VERKADA_PROFILE"), cf.CurrentProfile, "default")
	profile, ok := cf.Profiles[profileName]
	if !ok {
		return fmt.Errorf("profile %q not found in %s", profileName, p)
	}
	profile.OrgID = orgID
	cf.Profiles[profileName] = profile
	return writeConfig(p, cf)
}

var errNoBody = errors.New("no body provided")

func readBodyArg(s string) ([]byte, error) {
	if s == "" {
		return nil, errNoBody
	}
	if len(s) > 1 && s[0] == '@' {
		return os.ReadFile(s[1:])
	}
	return []byte(s), nil
}
