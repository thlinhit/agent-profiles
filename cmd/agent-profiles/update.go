package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

const updateUsage = "usage: update <agent> <name> | update <agent> --all"

// runUpdate refreshes ap-owned settings (such as the Claude status line) in
// existing profiles. User-owned settings like permissions and env are kept.
func runUpdate(s profile.Store, args []string, out io.Writer) error {
	if len(args) != 3 {
		return fmt.Errorf(updateUsage)
	}
	a, err := agent.Get(args[1])
	if err != nil {
		return err
	}
	u, ok := a.(agent.SettingsUpdater)
	if !ok {
		return fmt.Errorf("agent %q has no settings to update", a.Name())
	}
	names := []string{args[2]}
	if args[2] == "--all" {
		if names, err = s.List(a.Name()); err != nil {
			return err
		}
	}
	for _, name := range names {
		current, err := s.ReadSettings(a.Name(), name)
		if err != nil {
			return err
		}
		next, err := u.UpdateSettings(current)
		if err != nil {
			return fmt.Errorf("%s/%s: %w", a.Name(), name, err)
		}
		if !bytes.Equal(current, next) {
			if err := s.WriteSettings(a.Name(), name, next); err != nil {
				return err
			}
		}
		if err := syncSkillDirs(s, a, name); err != nil {
			return fmt.Errorf("%s/%s: %w", a.Name(), name, err)
		}
		final, err := s.ReadSettings(a.Name(), name)
		if err != nil {
			return err
		}
		status := "unchanged"
		if !bytes.Equal(current, final) {
			status = "updated"
		}
		if _, err := fmt.Fprintf(out, "%s/%s: %s\n", a.Name(), name, status); err != nil {
			return err
		}
	}
	return nil
}
