package dotfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func put(t *testing.T, p, s string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(s), 0o600); err != nil {
		t.Fatal(err)
	}
}
func TestSafeDotfileWrite(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	put(t, outside, "keep")
	path := filepath.Join(dir, Filename)
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, "claude", "work"); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("symlink not refused: %v", err)
	}
	got, _ := os.ReadFile(outside)
	if string(got) != "keep" {
		t.Fatal("symlink target clobbered")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, "claude", "work"); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, "claude", "personal"); err != nil {
		t.Fatal(err)
	}
	doc, err := Find(dir)
	if err != nil || doc.Profiles["claude"] != "personal" {
		t.Fatalf("roundtrip: %+v %v", doc, err)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0o600 {
		t.Fatal("dotfile not private")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatal("temporary files leaked")
	}
}
func TestDotfileReading(t *testing.T) {
	home := t.TempDir()
	home, _ = filepath.EvalSymlinks(home)
	t.Setenv("HOME", home)
	repo := filepath.Join(home, "repo")
	child := filepath.Join(repo, "sub")
	if err := os.MkdirAll(filepath.Join(repo, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(child, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(repo, Filename)
	put(t, path, "claude = \"work\"\n")
	doc, err := Find(child)
	if err != nil || doc.Dir != repo {
		t.Fatalf("find: %+v %v", doc, err)
	}
	put(t, path, strings.Repeat("x", 4097))
	if _, err := Find(child); err == nil {
		t.Fatal("oversized file accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(home, "outside")
	put(t, outside, "claude = \"work\"\n")
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(child); err == nil {
		t.Fatal("outside-repo symlink accepted")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	inside := filepath.Join(repo, "config")
	put(t, inside, "claude = \"work\"\n")
	if err := os.Symlink(inside, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Find(child); err != nil {
		t.Fatalf("internal link: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(home, Filename), "claude = \"personal\"\n")
	doc, err = Find(child)
	if err != nil || doc != nil {
		t.Fatalf("walked above repository: %+v %v", doc, err)
	}
	empty := filepath.Join(home, "empty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	doc, err = Find(empty)
	if err != nil || doc.Dir != home {
		t.Fatalf("home boundary: %+v %v", doc, err)
	}
	put(t, filepath.Join(filepath.Dir(home), Filename), "claude = \"above\"\n")
	t.Cleanup(func() { _ = os.Remove(filepath.Join(filepath.Dir(home), Filename)) })
	if err := os.Remove(filepath.Join(home, Filename)); err != nil {
		t.Fatal(err)
	}
	doc, err = Find(empty)
	if err != nil || doc != nil {
		t.Fatalf("walked above HOME: %+v %v", doc, err)
	}
}
func TestParse(t *testing.T) {
	for _, s := range []string{"claude = '../x'", "claude = 1", "claude = 'work'\nclaude = 'other'", "[claude]\nx='work'", "bad/key = 'work'"} {
		if _, err := Parse([]byte(s)); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	if p, err := Parse([]byte("# comment\nclaude = 'work' # comment\n")); err != nil || p["claude"] != "work" {
		t.Fatalf("valid TOML: %v %v", p, err)
	}
}
func FuzzParse(f *testing.F) {
	for _, s := range []string{"claude = \"work\"", "", "\xff", "claude = '../x'"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := Parse(b)
		if err == nil {
			for a, n := range p {
				if a == "" || n == "" {
					t.Fatal("empty name accepted")
				}
			}
		}
	})
}
