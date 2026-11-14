package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/config"
	"github.com/thlinhit/agent-profiles/internal/profile"
	"github.com/thlinhit/agent-profiles/internal/skills"
)

func runSkills(s profile.Store, args []string, out io.Writer) error {
	jsonOutput := false
	filtered := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	args = filtered
	if len(args) < 2 {
		return errors.New(skillsUsage())
	}
	sub, rest := args[1], args[2:]
	switch sub {
	case "pool":
		return runSkillPool(rest, out, jsonOutput)
	case "select":
		return runSkillSelect(s, rest, out)
	case "ls":
		if len(rest) > 0 && rest[0] == "--pool" {
			if len(rest) != 1 {
				return errors.New(skillUsage("ls --pool"))
			}
			return listPoolSkills(out, jsonOutput)
		}
	case "add", "rm":
	default:
		return unknownSkillSubcommand("", sub)
	}
	if !skillArgsValid(sub, args) {
		return errors.New(skillUsage(sub))
	}
	a, dir, err := resolveProfile(s, args[2], args[3])
	if err != nil {
		return err
	}
	profileDir := dir
	dir = filepath.Join(dir, a.SkillsSubdir())
	switch sub {
	case "ls":
		if jsonOutput {
			type item struct {
				Name   string `json:"name"`
				Target string `json:"target"`
				Kind   string `json:"kind"`
			}
			items := make([]item, 0)
			for link, e := range skills.Links(dir, nil) {
				if e != nil {
					return e
				}
				kind := "not_link"
				if link.Kind == skills.Broken {
					kind = "broken"
				} else if link.Kind != skills.NonLink {
					kind = "link"
				}
				items = append(items, item{link.Name, link.Target, kind})
			}
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(items)
		}
		if err := listSkills(dir, out); err != nil {
			return err
		}
		if hinter, ok := a.(agent.PluginSkillsHinter); ok {
			_, err = fmt.Fprintln(out, hinter.PluginSkillsHint())
		}
		return err
	case "add":
		source, err := skills.NewSource()(args[4])
		if err != nil {
			return err
		}
		name := filepath.Base(source)
		if len(args) == 7 {
			name = args[6]
		}
		if err := skills.Add(dir, source, name); err != nil {
			return err
		}
		return skills.SyncDirs(a, args[3], profileDir)
	default: // rm
		if err := skills.Remove(dir, args[4]); err != nil {
			return err
		}
		return skills.SyncDirs(a, args[3], profileDir)
	}
}

// skillArgsValid reports whether args (ap skills <sub> ...) has the arity of sub.
func skillArgsValid(sub string, args []string) bool {
	switch sub {
	case "ls":
		return len(args) == 4
	case "add":
		return len(args) == 5 || (len(args) == 7 && args[5] == "--name")
	case "rm":
		return len(args) == 5
	}
	return true
}

func listPoolSkills(out io.Writer, jsonOutput bool) error {
	found, _, err := skills.ScanPools(nil)
	if err != nil {
		return err
	}
	if jsonOutput {
		type item struct {
			Name string `json:"name"`
			Path string `json:"path"`
		}
		items := make([]item, 0, len(found))
		for _, sk := range found {
			items = append(items, item{filepath.Base(sk.Path), sk.Path})
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}
	for _, sk := range found {
		if _, err := fmt.Fprintf(out, "%s  %s\n", filepath.Base(sk.Path), sk.Path); err != nil {
			return err
		}
	}
	return nil
}

func listSkills(dir string, out io.Writer) error {
	for link, err := range skills.Links(dir, nil) {
		if err != nil {
			return err
		}
		line := link.Name + " (not a link)"
		if link.Kind != skills.NonLink {
			line = link.Name + " -> " + link.Target
			if link.Kind == skills.Broken {
				line += " (broken)"
			}
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func runSkillPool(args []string, out io.Writer, jsonOutput bool) error {
	if len(args) == 0 {
		return errors.New(skillUsage("pool"))
	}
	switch args[0] {
	case "add", "rm":
		if len(args) != 2 || args[1] == "" {
			return errors.New(skillUsage("pool " + args[0]))
		}
		return config.UpdatePool(args[1], args[0] == "add")
	case "ls":
		if len(args) != 1 {
			return errors.New(skillUsage("pool ls"))
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if jsonOutput {
			type item struct {
				Path    string `json:"path"`
				Missing bool   `json:"missing"`
			}
			items := make([]item, 0, len(cfg.SkillPools))
			for _, pool := range cfg.SkillPools {
				_, err := os.Stat(pool)
				if err != nil && !os.IsNotExist(err) {
					return err
				}
				items = append(items, item{pool, os.IsNotExist(err)})
			}
			enc := json.NewEncoder(out)
			enc.SetIndent("", "  ")
			return enc.Encode(items)
		}
		for _, pool := range cfg.SkillPools {
			suffix := ""
			if _, err := os.Stat(pool); os.IsNotExist(err) {
				suffix = " (missing)"
			} else if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(out, pool+suffix); err != nil {
				return err
			}
		}
		return nil
	default:
		return unknownSkillSubcommand("pool", args[0])
	}
}
