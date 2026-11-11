package profile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePreservesCheckDirErrors(t *testing.T) {
	for _, kind := range []string{"symlink", "non-directory", "permissive", "permission denied"} {
		t.Run(kind, func(t *testing.T) {
			s := setup(t)
			path, err := s.Create("pi", "work", nil)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "symlink", "non-directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink" {
					err = os.Symlink(filepath.Join(t.TempDir(), "missing-target"), path)
				} else {
					err = os.WriteFile(path, []byte("not a directory"), 0o600)
				}
			case "permissive":
				err = os.Chmod(path, 0o755)
			case "permission denied":
				if os.Geteuid() == 0 {
					t.Skip("root bypasses directory search permissions")
				}
				parent := filepath.Dir(path)
				t.Cleanup(func() {
					if err := os.Chmod(parent, 0o700); err != nil {
						t.Error(err)
					}
				})
				err = os.Chmod(parent, 0)
			}
			if err != nil {
				t.Fatal(err)
			}
			want := CheckDir(path)
			if want == nil {
				t.Fatal("fixture did not produce a CheckDir error")
			}
			got, err := s.Resolve("pi", "work")
			if err == nil || err.Error() != want.Error() || got != "" {
				t.Fatalf("Resolve() = %q, %v; want unchanged error %v", got, err, want)
			}
		})
	}
}
