// Package config loads agent-profiles configuration.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config contains the configured skill pool paths.
type Config struct {
	SkillPools []string `toml:"skill_pools"`
	MCPPool    string   `toml:"mcp_pool"`
}

func configDir() (string, error) {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve config home: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "agent-profiles"), nil
}

func configPath() (string, error) {
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// MCPPoolPath returns the MCP pool file: the configured mcp_pool if set,
// otherwise mcp.json in the agent-profiles config directory.
func MCPPoolPath() (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}
	if cfg.MCPPool != "" {
		return cfg.MCPPool, nil
	}
	dir, err := configDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "mcp.json"), nil
}

// Load reads config.toml from XDG_CONFIG_HOME or ~/.config.
// A missing configuration file is treated as an empty configuration.
func Load() (Config, error) {
	path, err := configPath()
	if err != nil {
		return Config{}, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}
	for i, pool := range cfg.SkillPools {
		expanded, err := ExpandHome(pool)
		if err != nil {
			return Config{}, fmt.Errorf("expand skill_pools[%d]: %w", i, err)
		}
		cfg.SkillPools[i] = expanded
	}
	mcpPool, err := ExpandHome(cfg.MCPPool)
	if err != nil {
		return Config{}, fmt.Errorf("expand mcp_pool: %w", err)
	}
	cfg.MCPPool = mcpPool
	return cfg, nil
}

// ExpandHome replaces a leading ~/ with the current user's home directory.
// Other paths are returned unchanged.
func ExpandHome(path string) (string, error) {
	if !strings.HasPrefix(path, "~/") {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home directory: %w", err)
	}
	return filepath.Join(home, path[2:]), nil
}
