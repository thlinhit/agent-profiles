package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

func skillsFixture(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("HOME", base)
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("TMPDIR", t.TempDir())
	dir, err := (profile.Store{}).Create("claude", "work", nil)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(base, "source")
	makeSkill(t, source)
	return dir, source
}
func makeSkill(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("skill"), 0600); err != nil {
		t.Fatal(err)
	}
}
func skillsRun(args ...string) (string, error) {
	var out bytes.Buffer
	err := run(append([]string{"skills"}, args...), &out, &out)
	return out.String(), err
}
func TestSkillsAdd(t *testing.T) {
	for _, tc := range []string{"missing SKILL.md", "resolved link", "temporary source", "profile source", "skills symlink", "existing broken link", "invalid name", "nonregular SKILL.md"} {
		t.Run(tc, func(t *testing.T) {
			dir, source := skillsFixture(t)
			input := source
			name := "source"
			wantErr := tc != "resolved link"
			switch tc {
			case "missing SKILL.md":
				if err := os.Remove(filepath.Join(source, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			case "nonregular SKILL.md":
				if err := os.Remove(filepath.Join(source, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(filepath.Join(source, "SKILL.md"), 0700); err != nil {
					t.Fatal(err)
				}
			case "resolved link":
				input = filepath.Join(filepath.Dir(source), "alias")
				if err := os.Symlink(source, input); err != nil {
					t.Fatal(err)
				}
			case "temporary source":
				input = filepath.Join(os.TempDir(), "temporary")
				makeSkill(t, input)
			case "profile source":
				input = filepath.Join(dir, "nested")
				makeSkill(t, input)
			case "skills symlink":
				if err := os.Symlink(t.TempDir(), filepath.Join(dir, "skills")); err != nil {
					t.Fatal(err)
				}
			case "existing broken link":
				if err := os.Mkdir(filepath.Join(dir, "skills"), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink("missing", filepath.Join(dir, "skills", name)); err != nil {
					t.Fatal(err)
				}
			case "invalid name":
				name = "../bad"
			}
			_, err := skillsRun("add", "claude", "work", input, "--name", name)
			if wantErr {
				if err == nil {
					t.Fatal("expected refusal")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			target, err := os.Readlink(filepath.Join(dir, "skills", name))
			if err != nil {
				t.Fatal(err)
			}
			resolved, err := filepath.EvalSymlinks(source)
			if err != nil {
				t.Fatal(err)
			}
			if !filepath.IsAbs(target) || target != resolved {
				t.Fatalf("target = %q, want %q", target, resolved)
			}
			st, err := os.Stat(filepath.Join(dir, "skills"))
			if err != nil || st.Mode().Perm() != 0700 {
				t.Fatalf("skills permissions: %v %v", st, err)
			}
			if _, err := skillsRun("add", "claude", "work", source); err == nil || !strings.Contains(err.Error(), "skills rm") {
				t.Fatalf("duplicate: %v", err)
			}
		})
	}
}
func TestSkillPoolCommands(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprintf("missing=%v", missing), func(t *testing.T) {
			_, source := skillsFixture(t)
			if _, err := skillsRun("pool", "add", source); err != nil {
				t.Fatal(err)
			}
			if missing {
				if err := os.RemoveAll(source); err != nil {
					t.Fatal(err)
				}
			}
			out, err := skillsRun("pool", "ls")
			if err != nil {
				t.Fatal(err)
			}
			want := source
			if missing {
				want += " (missing)"
			}
			if out != want+"\n" {
				t.Fatalf("listing = %q, want %q", out, want)
			}
			if _, err := skillsRun("pool", "rm", source); err != nil {
				t.Fatal(err)
			}
			out, err = skillsRun("pool", "ls")
			if err != nil || out != "" {
				t.Fatalf("after rm = %q, %v", out, err)
			}
		})
	}
}

func TestSkillsExistingPermissions(t *testing.T) {
	dir, source := skillsFixture(t)
	skills := filepath.Join(dir, "skills")
	if err := os.Mkdir(skills, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(skills, 0755); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "claude", "work", source},
		{"ls", "claude", "work"},
		{"rm", "claude", "work", "source"},
	} {
		if _, err := skillsRun(args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		st, err := os.Lstat(skills)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0755 {
			t.Fatalf("skills permissions changed: %o", st.Mode().Perm())
		}
	}
	if !regularSkill(source) {
		t.Fatal("source damaged")
	}
}

func TestSkillsRemove(t *testing.T) {
	for _, kind := range []string{"link", "directory", "file", "skills symlink"} {
		t.Run(kind, func(t *testing.T) {
			dir, source := skillsFixture(t)
			skills := filepath.Join(dir, "skills")
			if err := os.Mkdir(skills, 0700); err != nil {
				t.Fatal(err)
			}
			dest := filepath.Join(skills, "source")
			switch kind {
			case "directory":
				makeSkill(t, dest)
			case "file":
				if err := os.WriteFile(dest, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			default:
				if err := os.Symlink(source, dest); err != nil {
					t.Fatal(err)
				}
				if kind == "skills symlink" {
					moved := filepath.Join(filepath.Dir(source), "outside")
					if err := os.Rename(skills, moved); err != nil {
						t.Fatal(err)
					}
					if err := os.Symlink(moved, skills); err != nil {
						t.Fatal(err)
					}
				}
			}
			_, err := skillsRun("rm", "claude", "work", "source")
			if kind == "link" {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Lstat(dest); !os.IsNotExist(err) {
					t.Fatalf("link remains: %v", err)
				}
			} else {
				if err == nil {
					t.Fatal("expected refusal")
				}
				if kind != "skills symlink" && !strings.Contains(err.Error(), "not managed by ap, delete it yourself") {
					t.Fatal(err)
				}
				if _, err := os.Lstat(dest); err != nil {
					t.Fatal(err)
				}
			}
			if !regularSkill(source) {
				t.Fatal("source damaged")
			}
		})
	}
}
func TestSkillsList(t *testing.T) {
	dir, _ := skillsFixture(t)
	skills := filepath.Join(dir, "skills")
	if err := os.Mkdir(skills, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing", filepath.Join(skills, "broken")); err != nil {
		t.Fatal(err)
	}
	makeSkill(t, filepath.Join(skills, "real"))
	out, err := skillsRun("ls", "claude", "work")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"broken -> missing (broken)", "real (not a link)", "(plugin skills: manage with /plugin inside this profile)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if _, err := (profile.Store{}).Create("pi", "work", nil); err != nil {
		t.Fatal(err)
	}
	out, err = skillsRun("ls", "pi", "work")
	if err != nil || out != "" {
		t.Fatalf("pi list = %q, %v", out, err)
	}
}
func TestSkillPoolLookup(t *testing.T) {
	for _, count := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			dir, source := skillsFixture(t)
			pool := filepath.Join(filepath.Dir(source), "pool")
			if err := os.Mkdir(pool, 0700); err != nil {
				t.Fatal(err)
			}
			var matches []string
			for i := 0; i < count; i++ {
				path := filepath.Join(pool, fmt.Sprint(i), "wanted")
				makeSkill(t, path)
				matches = append(matches, path)
			}
			// Neither ignored directories nor symlinked directories may participate.
			makeSkill(t, filepath.Join(pool, ".git", "wanted"))
			makeSkill(t, filepath.Join(pool, "node_modules", "wanted"))
			if err := os.Symlink(source, filepath.Join(pool, "alias")); err != nil {
				t.Fatal(err)
			}
			cfg := filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "agent-profiles")
			if err := os.MkdirAll(cfg, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cfg, "config.toml"), []byte(fmt.Sprintf("skill_pools = [%q]\n", pool)), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := skillsRun("add", "claude", "work", "wanted")
			if count == 1 {
				if err != nil {
					t.Fatal(err)
				}
				if _, err := os.Readlink(filepath.Join(dir, "skills", "wanted")); err != nil {
					t.Fatal(err)
				}
			} else {
				if err == nil {
					t.Fatal("expected lookup error")
				}
				wants := matches
				if count == 0 {
					wants = []string{pool}
				}
				for _, want := range wants {
					if !strings.Contains(err.Error(), want) {
						t.Fatalf("missing %s in %v", want, err)
					}
				}
			}
			out, err := skillsRun("ls", "--pool")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(out, "\n") != count {
				t.Fatalf("pool listing: %q", out)
			}
		})
	}
}
