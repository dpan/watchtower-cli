package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	content := `[[target]]
name = "prod"
url = "http://wt:8080"
token = "secret"
events_token = "events-secret"

[[target]]
name = "staging"
url = "http://staging:8080"
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != 2 {
		t.Fatalf("targets: %d", len(cfg.Targets))
	}

	tgt, err := cfg.Target("prod")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.URL != "http://wt:8080" || tgt.Token != "secret" || tgt.EventsToken != "events-secret" {
		t.Errorf("prod target: %+v", tgt)
	}

	tgt, err = cfg.Target("staging")
	if err != nil {
		t.Fatal(err)
	}
	if tgt.Token != "" {
		t.Errorf("staging token should be empty, got %q", tgt.Token)
	}
}

func TestLoadConfigEmptyPath(t *testing.T) {
	cfg, err := LoadConfig("")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Targets) != 0 {
		t.Errorf("targets: %+v", cfg.Targets)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	_, err := LoadConfig(filepath.Join(t.TempDir(), "nope.toml"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error: %v", err)
	}
}

func TestTargetUnknown(t *testing.T) {
	cfg := &Config{Targets: []Target{{Name: "a", URL: "http://a"}}}
	if _, err := cfg.Target("b"); err == nil {
		t.Error("expected error for unknown target")
	}
}

func TestConfigPathSearch(t *testing.T) {
	t.Setenv("WATCHTOWER_CLI_CONFIG", "")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(old) })

	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != "" {
		t.Errorf("expected no config path, got %q", path)
	}

	content := `[[target]]
name = "local"
url = "http://local:8080"
`
	if err := os.WriteFile(filepath.Join(dir, ConfigFileName), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	path, err = ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != ConfigFileName {
		t.Errorf("expected %q, got %q", ConfigFileName, path)
	}

	t.Setenv("WATCHTOWER_CLI_CONFIG", "/custom/targets.toml")
	path, err = ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if path != "/custom/targets.toml" {
		t.Errorf("expected env path, got %q", path)
	}
}

func TestPermissionWarning(t *testing.T) {
	path := filepath.Join(t.TempDir(), "targets.toml")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if msg := PermissionWarning(path); !strings.Contains(msg, "readable by group") {
		t.Errorf("0644: %q", msg)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if msg := PermissionWarning(path); msg != "" {
		t.Errorf("0600: %q", msg)
	}
	if msg := PermissionWarning(filepath.Join(t.TempDir(), "missing")); msg != "" {
		t.Errorf("missing: %q", msg)
	}
}
