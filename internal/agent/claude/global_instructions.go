package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"slices"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

// excludesKey is the Claude Code setting that skips CLAUDE.md files by
// absolute-path glob.
const excludesKey = "claudeMdExcludes"

// globalInstructionPaths are the instruction files Claude Code loads from the
// home directory, relative to it. They are excluded when Global instructions
// are off.
var globalInstructionPaths = []string{
	".claude/CLAUDE.md",
	".claude/rules/**",
	"AGENTS.md",
	".claude/AGENTS.md",
	"CLAUDE.md",
	"CLAUDE.local.md",
}

func excludePaths(home string) ([]string, error) {
	if !filepath.IsAbs(home) {
		return nil, fmt.Errorf("home directory %q is not an absolute path", home)
	}
	paths := make([]string, len(globalInstructionPaths))
	for i, p := range globalInstructionPaths {
		paths[i] = filepath.Join(home, p)
	}
	return paths, nil
}

func readExcludes(settings map[string]json.RawMessage) ([]string, error) {
	raw, ok := settings[excludesKey]
	if !ok {
		return nil, nil
	}
	notList := fmt.Errorf("parse settings: %s is not a string list", excludesKey)
	// null unmarshals into a nil slice without error, so reject it explicitly.
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return nil, notList
	}
	list := make([]string, 0, len(items))
	for _, item := range items {
		// null unmarshals into a string without error, so check the JSON kind first.
		var s string
		if len(item) == 0 || item[0] != '"' || json.Unmarshal(item, &s) != nil {
			return nil, notList
		}
		list = append(list, s)
	}
	return list, nil
}

// GlobalInstructionsState is on when none of the home instruction files are
// excluded, off when all are, and partial otherwise.
func (Adapter) GlobalInstructionsState(current []byte, home string) (profile.SettingState, error) {
	settings, err := parseSettings(current)
	if err != nil {
		return "", err
	}
	paths, err := excludePaths(home)
	if err != nil {
		return "", err
	}
	excludes, err := readExcludes(settings)
	if err != nil {
		return "", err
	}
	excluded := 0
	for _, p := range paths {
		if slices.Contains(excludes, p) {
			excluded++
		}
	}
	switch excluded {
	case 0:
		return profile.SettingOn, nil
	case len(paths):
		return profile.SettingOff, nil
	default:
		return profile.SettingPartial, nil
	}
}

// WithGlobalInstructions adds the home instruction files to claudeMdExcludes
// when off, or removes only those entries when on. User entries and other
// keys are kept; claudeMdExcludes is dropped when it ends up empty.
func (Adapter) WithGlobalInstructions(current []byte, home string, on bool) ([]byte, error) {
	settings, err := parseSettings(current)
	if err != nil {
		return nil, err
	}
	paths, err := excludePaths(home)
	if err != nil {
		return nil, err
	}
	excludes, err := readExcludes(settings)
	if err != nil {
		return nil, err
	}
	var next []string
	if on {
		for _, e := range excludes {
			if !slices.Contains(paths, e) {
				next = append(next, e)
			}
		}
	} else {
		next = slices.Clone(excludes)
		for _, p := range paths {
			if !slices.Contains(next, p) {
				next = append(next, p)
			}
		}
	}
	if len(next) == 0 {
		delete(settings, excludesKey)
	} else {
		raw, err := json.Marshal(next)
		if err != nil {
			return nil, err
		}
		settings[excludesKey] = raw
	}
	if len(settings) == 0 && len(current) == 0 {
		return nil, nil
	}
	return marshalSettings(settings)
}
