package cli

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestProfilesAddWritesNamedProfile(t *testing.T) {
	td := t.TempDir()
	cfgPath := filepath.Join(td, "config.json")

	cmd := NewRootCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{
		"profiles", "add", "work",
		"--no-prompt",
		"--no-verify",
		"--config", cfgPath,
		"--base-url", "https://api.example.com",
		"--org-id", "ORG123",
		"--api-key", "abc123",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr=%q)", err, errBuf.String())
	}

	cf, err := loadConfig(cfgPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cf.CurrentProfile != "work" {
		t.Fatalf("current_profile = %q", cf.CurrentProfile)
	}
	p := cf.Profiles["work"]
	if p.BaseURL != "https://api.example.com" {
		t.Fatalf("base_url = %q", p.BaseURL)
	}
	if p.OrgID != "ORG123" {
		t.Fatalf("org_id = %q", p.OrgID)
	}
	if p.Auth.APIKey != "abc123" {
		t.Fatalf("api_key = %q", p.Auth.APIKey)
	}
}

func TestProfilesListJSON(t *testing.T) {
	td := t.TempDir()
	cfgPath := filepath.Join(td, "config.json")

	orig := ConfigFile{
		CurrentProfile: "b",
		Profiles: map[string]Config{
			"a": {BaseURL: "https://api.a.example.com", Headers: map[string]string{}},
			"b": {BaseURL: "https://api.b.example.com", Headers: map[string]string{}},
		},
	}
	if err := writeConfig(cfgPath, orig); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := NewRootCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{
		"profiles", "list",
		"--config", cfgPath,
		"--output", "json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr=%q)", err, errBuf.String())
	}
	got := out.String()
	if !bytes.Contains([]byte(got), []byte(`"current_profile": "b"`)) {
		t.Fatalf("expected current_profile in output, got=%q", got)
	}
	if !bytes.Contains([]byte(got), []byte(`"name": "a"`)) || !bytes.Contains([]byte(got), []byte(`"name": "b"`)) {
		t.Fatalf("expected profiles a/b in output, got=%q", got)
	}
}

func TestProfilesPath(t *testing.T) {
	td := t.TempDir()
	cfgPath := filepath.Join(td, "config.json")

	orig := ConfigFile{
		CurrentProfile: "default",
		Profiles: map[string]Config{
			"default": {BaseURL: "https://api.example.com", Headers: map[string]string{}},
		},
	}
	if err := writeConfig(cfgPath, orig); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := NewRootCmd()
	var out, errBuf bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errBuf)
	cmd.SetArgs([]string{
		"profiles", "path",
		"--config", cfgPath,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v (stderr=%q)", err, errBuf.String())
	}
	if got := out.String(); got != cfgPath+"\n" {
		t.Fatalf("got=%q want=%q", got, cfgPath+"\n")
	}
}
