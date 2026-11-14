package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func runList(s profile.Store, args []string, out io.Writer) error {
	jsonOutput := false
	filtered := args[:0]
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	args = filtered
	if len(args) > 2 {
		return errors.New(commandUsage("list"))
	}
	agents := agent.All()
	if len(args) == 2 {
		a, err := agent.Get(args[1])
		if err != nil {
			return err
		}
		agents = []agent.Agent{a}
	}
	if jsonOutput {
		type item struct {
			Agent   string `json:"agent"`
			Profile string `json:"profile"`
			Path    string `json:"path"`
		}
		items := make([]item, 0)
		for _, a := range agents {
			names, err := s.List(a.Name())
			if err != nil {
				return err
			}
			for _, n := range names {
				path, err := s.Resolve(a.Name(), n)
				if err != nil {
					return err
				}
				abs, err := filepath.Abs(path)
				if err != nil {
					return err
				}
				items = append(items, item{a.Name(), n, abs})
			}
		}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}
	for _, a := range agents {
		names, err := s.List(a.Name())
		if err != nil {
			return err
		}
		for _, n := range names {
			if _, err := fmt.Fprintf(out, "%s %s\n", a.Name(), n); err != nil {
				return err
			}
		}
	}
	return nil
}
