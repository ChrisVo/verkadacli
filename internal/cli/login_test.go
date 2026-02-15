package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoginWritesConfig(t *testing.T) {
	td := t.TempDir()
	cfgPath := filepath.Join(td, "config.json")

	cmd := NewRootCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{
		"login",
		"--no-prompt",
		"--config", cfgPath,
		"--base-url", "https://api.example.com",
		"--api-key", "abc123",
	})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr=%q)", err, errBuf.String())
	}

	cf, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cf.CurrentProfile != "default" {
		t.Fatalf("current_profile = %q", cf.CurrentProfile)
	}
	p := cf.Profiles["default"]
	if p.BaseURL != "https://api.example.com" {
		t.Fatalf("base_url = %q", p.BaseURL)
	}
	if p.Auth.APIKey != "abc123" {
		t.Fatalf("api_key = %q", p.Auth.APIKey)
	}
}

func TestLoginPreservesHeaders(t *testing.T) {
	td := t.TempDir()
	cfgPath := filepath.Join(td, "config.json")

	orig := ConfigFile{
		CurrentProfile: "default",
		Profiles: map[string]Config{
			"default": {
				BaseURL: "https://api.old.example.com",
				Auth: AuthConfig{
					APIKey: "old",
				},
				Headers: map[string]string{
					"X-Foo": "bar",
				},
			},
		},
	}
	if err := writeConfig(cfgPath, orig); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"login",
		"--no-prompt",
		"--config", cfgPath,
		"--base-url", "https://api.new.example.com",
		"--api-key", "newkey",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	cf, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cf.Profiles["default"].Headers["X-Foo"] != "bar" {
		t.Fatalf("headers preserved = %v", cf.Profiles["default"].Headers)
	}
}

func TestLoginNoPromptRequiresAPIKey(t *testing.T) {
	td := t.TempDir()
	cfgPath := filepath.Join(td, "config.json")

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"login",
		"--no-prompt",
		"--config", cfgPath,
		"--base-url", "https://api.example.com",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error")
	}

	if _, err := os.Stat(cfgPath); err == nil {
		t.Fatalf("config should not be written on failure")
	}
}

func TestLoginRejectsCommandWebURL(t *testing.T) {
	td := t.TempDir()
	cfgPath := filepath.Join(td, "config.json")

	cmd := NewRootCmd()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{
		"login",
		"--no-prompt",
		"--config", cfgPath,
		"--base-url", "https://st-hedwig-church.command.verkada.com/cameras",
		"--api-key", "abc123",
	})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSanitizeBaseURLDefault(t *testing.T) {
	in := "https://st-hedwig-church.command.verkada.com/cameras"
	if got := sanitizeBaseURLDefault(in); got != "https://api.verkada.com" {
		t.Fatalf("got %q", got)
	}
}
