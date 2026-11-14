package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// currentItem is one Agent's Active profile and its Source.
type currentItem struct {
	Agent   string  `json:"agent"`
	Path    string  `json:"path"`
	Source  string  `json:"source"`
	Profile *string `json:"profile,omitempty"`
}

func runCurrent(args []string, out, diagnostic io.Writer) error {
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	if len(filtered) != 1 {
		return errors.New(commandUsage("current"))
	}
	trusted, err := selections(diagnostic)
	if err != nil {
		return err
	}
	items := make([]currentItem, 0, len(agent.All()))
	for _, a := range agent.All() {
		item, err := activeProfile(a, trusted)
		if err != nil {
			return err
		}
		items = append(items, item)
	}
	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}
	for _, item := range items {
		if _, err := fmt.Fprintf(out, "%s %s (%s)\n", item.Agent, item.Path, item.Source); err != nil {
			return err
		}
	}
	return nil
}

// activeProfile reports what a reads right now. The env var wins when set; its Source is
// dotfile only if it equals the trusted Selection's path. When unset, the Default config dir
// is active and any trusted Dotfile is ignored.
func activeProfile(a agent.Agent, trusted map[string]string) (currentItem, error) {
	item := currentItem{Agent: a.Name()}
	if env := os.Getenv(a.EnvVar()); env != "" {
		item.Path, item.Source = env, "env"
		if p, ok := trusted[a.Name()]; ok && filepath.Clean(env) == filepath.Clean(p) {
			item.Source = "dotfile"
		}
	} else {
		dir, err := a.DefaultConfigDir()
		if err != nil {
			return item, err
		}
		item.Path, item.Source = dir, "default"
	}
	item.Profile = profileName(a.Name(), item.Path)
	return item, nil
}

// profileName returns the Profile name when dir lies inside the Agent's profile storage.
func profileName(agentName, dir string) *string {
	root, err := profile.DataDir()
	if err != nil {
		return nil
	}
	rel, err := filepath.Rel(filepath.Join(root, agentName), dir)
	if err != nil || !filepath.IsLocal(rel) || rel == "." || strings.HasPrefix(rel, "..") {
		return nil
	}
	return &rel
}
