package trust

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/dotfile"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func TestPerDirectoryTrust(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	dir := filepath.Join(home, "repo")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := dotfile.Write(dir, "claude", "work"); err != nil {
		t.Fatal(err)
	}
	doc, err := dotfile.Find(dir)
	if err != nil {
		t.Fatal(err)
	}
	s := Store{}
	if ok, err := s.Trusted(doc); err != nil || ok {
		t.Fatalf("implicitly trusted: %v %v", ok, err)
	}
	if err := s.Grant(doc); err != nil {
		t.Fatal(err)
	}
	if ok, err := (Store{}).Trusted(doc); err != nil || !ok {
		t.Fatalf("grant not persisted: %v %v", ok, err)
	}
	root, _ := profile.DataDir()
	st, err := os.Stat(filepath.Join(root, "trust.json"))
	if err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("trust permissions: %v %v", st, err)
	}
	if err := dotfile.Write(dir, "claude", "other"); err != nil {
		t.Fatal(err)
	}
	changed, _ := dotfile.Find(dir)
	if ok, err := s.Trusted(changed); err != nil || ok {
		t.Fatalf("changed content trusted: %v %v", ok, err)
	}
	other := *doc
	other.Dir = t.TempDir()
	if ok, err := s.Trusted(&other); err != nil || ok {
		t.Fatalf("other directory trusted: %v %v", ok, err)
	}
	if err := s.Revoke(doc.Dir); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.Trusted(doc); err != nil || ok {
		t.Fatalf("revoke failed: %v %v", ok, err)
	}
}
func TestTrustRejectsSymlinkDatabase(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", home)
	root, _ := profile.DataDir()
	if err := profile.PrivateDir(root); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "trust.json")); err != nil {
		t.Fatal(err)
	}
	if err := (Store{}).Grant(&dotfile.Document{Dir: home, Content: []byte("claude='work'")}); err == nil {
		t.Fatal("followed trust database symlink")
	}
	got, _ := os.ReadFile(outside)
	if string(got) != "{}" {
		t.Fatal("clobbered target")
	}
}
