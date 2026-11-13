package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// skillDirsFile records which Linked-skill access entries ap added, so a later
// sync removes only its own entries and never ones the user added.
const skillDirsFile = ".ap-skill-dirs.json"

// syncSkillDirs adds Linked-skill access for the real directories behind the
// profile's Skill links. Agents without SkillDirsSetter are left alone.
func syncSkillDirs(s profile.Store, a agent.Agent, name string) error {
	setter, ok := a.(agent.SkillDirsSetter)
	if !ok {
		return nil
	}
	dir, err := s.Resolve(a.Name(), name)
	if err != nil {
		return err
	}
	next, err := skillTargets(filepath.Join(dir, a.SkillsSubdir()))
	if err != nil {
		return err
	}
	var prev []string
	if data, err := os.ReadFile(filepath.Join(dir, skillDirsFile)); err == nil {
		if err := json.Unmarshal(data, &prev); err != nil {
			return err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	current, err := s.ReadSettings(a.Name(), name)
	if err != nil {
		return err
	}
	updated, err := setter.SetSkillDirs(current, prev, next)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, updated) {
		if err := s.WriteSettings(a.Name(), name, updated); err != nil {
			return err
		}
	}
	data, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, skillDirsFile), data, 0o600)
}

// skillTargets returns the sorted resolved targets of the symlinks in dir.
// Broken links and real directories are skipped.
func skillTargets(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	targets := []string{}
	for _, e := range entries {
		if e.Type()&os.ModeSymlink == 0 {
			continue
		}
		real, err := filepath.EvalSymlinks(filepath.Join(dir, e.Name()))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if !seen[real] {
			seen[real] = true
			targets = append(targets, real)
		}
	}
	sort.Strings(targets)
	return targets, nil
}
