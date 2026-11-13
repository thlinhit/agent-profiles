package main

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/config"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func runSkills(s profile.Store, args []string, out io.Writer) error {
	if len(args) > 1 && args[1] == "pool" {
		return runSkillPool(args[2:], out)
	}
	if len(args) == 3 && args[1] == "ls" && args[2] == "--pool" {
		pools, err := skillPools()
		if err != nil {
			return err
		}
		for _, path := range pools {
			if _, err := fmt.Fprintf(out, "%s  %s\n", filepath.Base(path), path); err != nil {
				return err
			}
		}
		return nil
	}
	if len(args) > 1 && args[1] == "select" {
		return runSkillSelect(s, args[2:], out)
	}
	if len(args) < 4 {
		return fmt.Errorf("usage: skills ls|add|rm|select <agent> <profile> [<skill>] [--name N], or skills ls --pool")
	}
	a, err := agent.Get(args[2])
	if err != nil {
		return err
	}
	dir, err := s.Resolve(a.Name(), args[3])
	if err != nil {
		return err
	}
	dir = filepath.Join(dir, a.SkillsSubdir())
	switch args[1] {
	case "ls":
		if len(args) != 4 {
			return fmt.Errorf("usage: skills ls <agent> <profile>")
		}
		if err := listSkills(dir, out); err != nil {
			return err
		}
		if a.Name() == "claude" {
			_, err = fmt.Fprintln(out, "(plugin skills: manage with /plugin inside this profile)")
		}
		return err
	case "add":
		if len(args) != 5 && (len(args) != 7 || args[5] != "--name") {
			return fmt.Errorf("usage: skills add <agent> <profile> <path-or-name> [--name N]")
		}
		source, err := skillSource(args[4])
		if err != nil {
			return err
		}
		name := filepath.Base(source)
		if len(args) == 7 {
			name = args[6]
		}
		return addSkill(dir, source, name)
	case "rm":
		if len(args) != 5 {
			return fmt.Errorf("usage: skills rm <agent> <profile> <skill>")
		}
		return removeSkill(dir, args[4])
	default:
		return fmt.Errorf("unknown skills command %q", args[1])
	}
}

// addSkill links a canonical skill source into a profile skills directory.
func addSkill(dir, source, name string) error {
	if err := profile.ValidateName(name); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := checkSkillsDir(dir); err != nil {
		return err
	}
	dest := filepath.Join(dir, name)
	if _, err := os.Lstat(dest); err == nil {
		return fmt.Errorf("skill %q already exists; run skills rm first", name)
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(source, dest); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("skill %q already exists; run skills rm first", name)
		}
		return err
	}
	return nil
}

// removeSkill deletes a skill entry only when it is a symlink managed by ap.
func removeSkill(dir, name string) error {
	if err := profile.ValidateName(name); err != nil {
		return err
	}
	if err := checkSkillsDir(dir); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	st, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink == 0 {
		return fmt.Errorf("%s: not managed by ap, delete it yourself", path)
	}
	return os.Remove(path)
}

// Skills directories may be created by the agent with non-private permissions.
// Profile directory checks remain in Store.Resolve.
func checkSkillsDir(dir string) error {
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return fmt.Errorf("not a real directory: %s", dir)
	}
	return nil
}

func listSkills(dir string, out io.Writer) error {
	if _, err := os.Lstat(dir); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if err := checkSkillsDir(dir); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		line := entry.Name() + " (not a link)"
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			line = entry.Name() + " -> " + target
			if _, err := os.Stat(path); os.IsNotExist(err) {
				line += " (broken)"
			} else if err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}
	return nil
}

func runSkillPool(args []string, out io.Writer) error {
	const usage = "usage: skills pool add|rm <path>, or skills pool ls"
	if len(args) == 0 {
		return fmt.Errorf("%s", usage)
	}
	switch args[0] {
	case "add", "rm":
		if len(args) != 2 || args[1] == "" {
			return fmt.Errorf("%s", usage)
		}
		return config.UpdatePool(args[1], args[0] == "add")
	case "ls":
		if len(args) != 1 {
			return fmt.Errorf("%s", usage)
		}
		cfg, err := config.Load()
		if err != nil {
			return err
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
		return fmt.Errorf("%s", usage)
	}
}

func regularSkill(dir string) bool {
	st, err := os.Lstat(filepath.Join(dir, "SKILL.md"))
	return err == nil && st.Mode().IsRegular()
}

// poolSkill is a skill directory discovered in a configured pool.
type poolSkill struct {
	Path string
	Pool string
}

func skillPools() ([]string, error) {
	skills, err := scanPools(nil)
	if err != nil {
		return nil, err
	}
	paths := make([]string, len(skills))
	for i, sk := range skills {
		paths[i] = sk.Path
	}
	return paths, nil
}

// scanPools walks configured pools. With warn nil, any walk error fails the
// scan; otherwise the failing pool is reported through warn and skipped.
func scanPools(warn func(error)) ([]poolSkill, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	var skills []poolSkill
	for _, pool := range cfg.SkillPools {
		var found []poolSkill
		err := filepath.WalkDir(pool, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() {
				return nil
			}
			if d.Name() == ".git" || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if regularSkill(path) {
				found = append(found, poolSkill{Path: path, Pool: pool})
			}
			return nil
		})
		if err != nil {
			err = fmt.Errorf("walk skill pool %s: %w", pool, err)
			if warn == nil {
				return nil, err
			}
			warn(err)
			continue
		}
		skills = append(skills, found...)
	}
	sort.Slice(skills, func(i, j int) bool {
		a, b := skills[i].Path, skills[j].Path
		if filepath.Base(a) == filepath.Base(b) {
			return a < b
		}
		return filepath.Base(a) < filepath.Base(b)
	})
	return skills, nil
}

func skillSource(input string) (string, error) {
	source := input
	if !strings.ContainsAny(input, string(os.PathSeparator)) && !strings.HasPrefix(input, ".") && !strings.HasPrefix(input, "~") {
		paths, err := skillPools()
		if err != nil {
			return "", err
		}
		var matches []string
		for _, path := range paths {
			if filepath.Base(path) == input {
				matches = append(matches, path)
			}
		}
		if len(matches) == 0 {
			cfg, err := config.Load()
			if err != nil {
				return "", err
			}
			return "", fmt.Errorf("skill %q not found; pools searched: %v", input, cfg.SkillPools)
		}
		if len(matches) > 1 {
			return "", fmt.Errorf("multiple matches for %q: %s; use a full path", input, strings.Join(matches, ", "))
		}
		source = matches[0]
	}
	source, err := config.ExpandHome(source)
	if err != nil {
		return "", err
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return "", err
	}
	source, err = filepath.EvalSymlinks(source)
	if err != nil {
		return "", err
	}
	st, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	if !st.IsDir() || !regularSkill(source) {
		return "", fmt.Errorf("skill source must be a directory containing a regular file SKILL.md: %s", source)
	}
	data, err := profile.DataDir()
	if err != nil {
		return "", err
	}
	for _, forbidden := range []string{data, os.TempDir()} {
		resolved, err := resolveSkillBoundary(forbidden)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(resolved, source)
		if err != nil {
			return "", err
		}
		if rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))) {
			return "", fmt.Errorf("skill source must not be under %s", forbidden)
		}
	}
	return source, nil
}

// Resolve existing ancestors too, including when the data directory is absent.
func resolveSkillBoundary(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return resolved, nil
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolved, err = resolveSkillBoundary(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(path)), nil
}
