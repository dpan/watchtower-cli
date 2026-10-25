package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// ConfigFileName is the fixed default config file name.
const ConfigFileName = "targets.toml"

// Target is a named Watchtower HTTP API endpoint.
type Target struct {
	Name        string `toml:"name"`
	URL         string `toml:"url"`
	Token       string `toml:"token"`
	EventsToken string `toml:"events_token"`
}

// Config is the watchtower-cli configuration file.
type Config struct {
	Targets []Target `toml:"target"`
}

// ConfigPath searches for the config file. It returns the first existing file
// from: $WATCHTOWER_CLI_CONFIG, ./targets.toml, and ~/.config/watchtower-cli/targets.toml.
// If WATCHTOWER_CLI_CONFIG is set, that path is returned even if the file is missing.
func ConfigPath() (string, error) {
	if env := os.Getenv("WATCHTOWER_CLI_CONFIG"); env != "" {
		return env, nil
	}

	for _, candidate := range []string{ConfigFileName, xdgConfigPath()} {
		if candidate == "" {
			continue
		}
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", nil
}

func xdgConfigPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "watchtower-cli", ConfigFileName)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "watchtower-cli", ConfigFileName)
}

// PermissionWarning reports when path is readable by group or others.
// An empty string means the file is owner-only, or it could not be stated.
func PermissionWarning(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return ""
	}
	perm := info.Mode().Perm()
	if perm&0o077 == 0 {
		return ""
	}
	return fmt.Sprintf("warning: %s is readable by group or others (mode %#o); it holds API tokens", path, perm)
}

// LoadConfig reads the config file at path. An empty path returns an empty
// config without error. A missing file at a non-empty path is an error.
func LoadConfig(path string) (*Config, error) {
	if path == "" {
		return &Config{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		return nil, fmt.Errorf("reading config %s: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %s: %w", path, err)
	}
	return &cfg, nil
}

// Target finds a target by name.
func (c *Config) Target(name string) (*Target, error) {
	for i := range c.Targets {
		if c.Targets[i].Name == name {
			return &c.Targets[i], nil
		}
	}
	return nil, fmt.Errorf("unknown target %q (config has: %s)", name, c.targetNames())
}

func (c *Config) targetNames() string {
	if len(c.Targets) == 0 {
		return "none"
	}
	names := make([]string, 0, len(c.Targets))
	for _, t := range c.Targets {
		names = append(names, t.Name)
	}
	return strings.Join(names, ", ")
}
