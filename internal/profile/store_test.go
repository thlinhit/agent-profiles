package profile

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func setup(t *testing.T) Store {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	return Store{}
}
func TestStrictNames(t *testing.T) {
	s := setup(t)
	for _, name := range []string{"", "../x", "-x", "a/b", "x y", "x\n", "é", strings.Repeat("a", 65)} {
		if _, err := s.Create("claude", name, nil); err == nil {
			t.Errorf("accepted %q", name)
		}
	}
	for _, name := range []string{"A", "a-b_0", strings.Repeat("a", 64)} {
		if err := ValidateName(name); err != nil {
			t.Error(err)
		}
	}
	if _, err := s.Create("../evil", "ok", nil); err == nil {
		t.Fatal("invalid agent accepted")
	}
}
func TestPrivatePermissions(t *testing.T) {
	s := setup(t)
	old := syscall.Umask(0o022)
	defer syscall.Umask(old)
	parent := os.Getenv("XDG_DATA_HOME")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := s.Create("claude", "work", []byte("{}\n"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{filepath.Dir(filepath.Dir(p)), filepath.Dir(p), p} {
		st, err := os.Stat(path)
		if err != nil || st.Mode().Perm() != 0o700 {
			t.Fatalf("%s: %v %v", path, st, err)
		}
	}
	st, err := os.Stat(filepath.Join(p, "settings.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("settings: %v %v", st, err)
	}
	st, _ = os.Stat(parent)
	if st.Mode().Perm() != 0o755 {
		t.Fatal("ancestor chmodded")
	}
	if _, err := s.Create("claude", "work", []byte("changed")); err == nil {
		t.Fatal("overwrote existing profile")
	}
}
func TestStoreLifecycle(t *testing.T) {
	s := setup(t)
	if list, err := s.List("claude"); err != nil || len(list) != 0 {
		t.Fatalf("empty: %v %v", list, err)
	}
	p, err := s.Create("claude", "work", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := s.Resolve("claude", "work"); err != nil || got != p {
		t.Fatalf("resolve %q %v", got, err)
	}
	list, err := s.List("claude")
	if err != nil || len(list) != 1 || list[0] != "work" {
		t.Fatalf("list %v %v", list, err)
	}
	if err := s.Delete("claude", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve("claude", "work"); err == nil {
		t.Fatal("deleted profile resolves")
	}
}
func TestDeleteNeverFollowsSymlinks(t *testing.T) {
	s := setup(t)
	p, err := s.Create("claude", "work", nil)
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "keep")
	if err := os.WriteFile(sentinel, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(p, "link")); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("claude", "work"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, p); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve("claude", "work"); err == nil {
		t.Fatal("resolved symlink")
	}
	if err := s.Delete("claude", "work"); err == nil {
		t.Fatal("accepted symlink profile")
	}
}
