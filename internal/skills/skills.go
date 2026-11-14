package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/thlinhit/agent-profiles/internal/config"
	"github.com/thlinhit/agent-profiles/internal/dotfile"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// Add links a canonical skill source into a profile skills directory.
func Add(dir, source, name string) error {
	if err := profile.ValidateName(name); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := checkDir(dir); err != nil {
		return err
	}
	dest := filepath.Join(dir, name)
	if err := os.Symlink(source, dest); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("skill %q already exists; run skills rm first", name)
		}
		return err
	}
	return nil
}

// Remove deletes a skill entry only when it is a symlink managed by ap.
func Remove(dir, name string) error {
	if err := profile.ValidateName(name); err != nil {
		return err
	}
	if err := checkDir(dir); err != nil {
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
func checkDir(dir string) error {
	st, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
		return fmt.Errorf("not a real directory: %s", dir)
	}
	return nil
}

func checkOptionalDir(dir string) error {
	if err := checkDir(dir); !os.IsNotExist(err) {
		return err
	}
	return nil
}

func IsSkill(dir string) bool {
	st, err := os.Lstat(filepath.Join(dir, "SKILL.md"))
	return err == nil && st.Mode().IsRegular()
}

// PoolSkill is a skill directory discovered in a configured pool.
type PoolSkill struct {
	Path string
	Pool string
}

// ScanPools walks configured pools. With warn nil, any walk error fails the
// scan; otherwise the failing pool is reported through warn and skipped.
func ScanPools(warn func(error)) ([]PoolSkill, []string, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, nil, err
	}
	var skills []PoolSkill
	for _, pool := range cfg.SkillPools {
		var found []PoolSkill
		err := filepath.WalkDir(pool, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == ".git" || d.Name() == "node_modules") {
				return filepath.SkipDir
			}
			if !d.IsDir() && d.Name() == "SKILL.md" && d.Type().IsRegular() {
				found = append(found, PoolSkill{Path: filepath.Dir(path), Pool: pool})
			}
			return nil
		})
		if err != nil {
			err = fmt.Errorf("walk skill pool %s: %w", pool, err)
			if warn == nil {
				return nil, nil, err
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
	return skills, cfg.SkillPools, nil
}

type skillBoundary struct{ path, resolved string }

// NewSource caches forbidden boundaries for one command, resolving each
// lazily so validation errors retain their existing order.
func NewSource() func(string) (string, error) {
	data := sync.OnceValues(func() (skillBoundary, error) {
		path, err := profile.DataDir()
		if err != nil {
			return skillBoundary{}, err
		}
		resolved, err := resolveSkillBoundary(path)
		return skillBoundary{path: path, resolved: resolved}, err
	})
	temp := sync.OnceValues(func() (skillBoundary, error) {
		path := os.TempDir()
		resolved, err := resolveSkillBoundary(path)
		return skillBoundary{path: path, resolved: resolved}, err
	})
	return func(input string) (string, error) {
		source := input
		if !strings.ContainsAny(input, string(os.PathSeparator)) && !strings.HasPrefix(input, ".") && !strings.HasPrefix(input, "~") {
			skills, pools, err := ScanPools(nil)
			if err != nil {
				return "", err
			}
			var matches []string
			for _, sk := range skills {
				if filepath.Base(sk.Path) == input {
					matches = append(matches, sk.Path)
				}
			}
			if len(matches) == 0 {
				return "", fmt.Errorf("skill %q not found; pools searched: %v", input, pools)
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
		source, err = dotfile.Canonical(source)
		if err != nil {
			return "", err
		}
		st, err := os.Stat(source)
		if err != nil {
			return "", err
		}
		if !st.IsDir() || !IsSkill(source) {
			return "", fmt.Errorf("skill source must be a directory containing a regular file SKILL.md: %s", source)
		}
		for _, boundary := range []func() (skillBoundary, error){data, temp} {
			forbidden, err := boundary()
			if err != nil {
				return "", err
			}
			rel, err := filepath.Rel(forbidden.resolved, source)
			if err != nil {
				return "", err
			}
			if filepath.IsLocal(rel) {
				return "", fmt.Errorf("skill source must not be under %s", forbidden.path)
			}
		}
		return source, nil
	}

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
