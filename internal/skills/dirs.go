package skills

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/fsutil"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// skillDirsFile records which Linked-skill access entries ap added, so a later
// sync removes only its own entries and never ones the user added.
const skillDirsFile = ".ap-skill-dirs.json"

// SyncDirs adds Linked-skill access in an already resolved profile directory.
// Agents without SkillDirsSetter are left alone.
func SyncDirs(a agent.Agent, name, dir string) error {
	if _, ok := a.(agent.SkillDirsSetter); !ok {
		return nil
	}
	current, err := profile.ReadSettings(dir, a.Name()+"/"+name)
	if err != nil {
		return err
	}
	updated, ledger, err := PrepareDirs(a, dir, current)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, updated) {
		if err := profile.WriteSettings(dir, a.Name()+"/"+name, updated); err != nil {
			return err
		}
	}
	return WriteDirsLedger(dir, ledger)
}

// PrepareDirs computes settings and a changed ledger without writing either.
func PrepareDirs(a agent.Agent, dir string, current []byte) ([]byte, []byte, error) {
	setter, ok := a.(agent.SkillDirsSetter)
	if !ok {
		return current, nil, nil
	}
	next, err := skillTargets(filepath.Join(dir, a.SkillsSubdir()))
	if err != nil {
		return nil, nil, err
	}
	var prev []string
	ledger, err := os.ReadFile(filepath.Join(dir, skillDirsFile))
	if err == nil {
		if err := json.Unmarshal(ledger, &prev); err != nil {
			return nil, nil, err
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, nil, err
	}
	updated, err := setter.SetSkillDirs(current, prev, next)
	if err != nil {
		return nil, nil, err
	}
	data, err := json.Marshal(next)
	if err != nil {
		return nil, nil, err
	}
	if bytes.Equal(ledger, data) {
		data = nil
	}
	return updated, data, nil
}

func WriteDirsLedger(dir string, data []byte) error {
	if data == nil {
		return nil
	}
	return fsutil.AtomicWrite(filepath.Join(dir, skillDirsFile), data)
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
		targets = append(targets, real)
	}
	slices.Sort(targets)
	return slices.Compact(targets), nil
}
