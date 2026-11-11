package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateExistingProfile(t *testing.T) {
	for _, a := range []string{"claude", "pi"} {
		t.Run(a, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
			var out, diagnostic bytes.Buffer
			if err := run([]string{"create", a, "work", "--init"}, &out, &diagnostic); err != nil {
				t.Fatal(err)
			}
			path := strings.TrimSpace(out.String())
			before, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			settings := filepath.Join(path, "settings.json")
			original := []byte("{\"keep\":true}\n")
			if err := os.WriteFile(settings, original, 0o600); err != nil {
				t.Fatal(err)
			}
			for _, emit := range []bool{false, true} {
				out.Reset()
				diagnostic.Reset()
				args := []string{"create", a, "work", "--init"}
				if emit {
					args = append(args, "--emit-shell")
				}
				err := run(args, &out, &diagnostic)
				want := `profile "` + a + `/work" already exists`
				if err == nil || err.Error() != want {
					t.Fatalf("create existing: %v; want %s", err, want)
				}
				if out.Len() != 0 || diagnostic.Len() != 0 {
					t.Fatalf("failed create emitted output: %q %q", out.String(), diagnostic.String())
				}
				got, err := os.ReadFile(settings)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatalf("existing settings changed: %q %v", got, err)
				}
				after, err := os.Stat(path)
				if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() {
					t.Fatalf("existing directory replaced or changed: %v", err)
				}
			}
		})
	}
}
