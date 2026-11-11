package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMissingProfileCommands(t *testing.T) {
	for _, a := range []string{"claude", "pi"} {
		for _, state := range []string{"missing store", "missing agent", "missing profile"} {
			t.Run(a+"/"+state, func(t *testing.T) {
				data := filepath.Join(t.TempDir(), "data")
				t.Setenv("XDG_DATA_HOME", data)
				t.Chdir(t.TempDir())
				if state != "missing store" {
					path := filepath.Join(data, "agent-profiles")
					if state == "missing profile" {
						path = filepath.Join(path, a)
					}
					if err := os.MkdirAll(path, 0o700); err != nil {
						t.Fatal(err)
					}
				}
				for _, command := range []string{"use", "delete", "local"} {
					for _, emit := range []bool{false, true} {
						var out, diagnostic bytes.Buffer
						args := []string{command, a, "missing"}
						if emit {
							args = append(args, "--emit-shell")
						}
						err := run(args, &out, &diagnostic)
						want := `profile "` + a + `/missing" does not exist; run ap list`
						if err == nil || err.Error() != want {
							t.Fatalf("%v: %v; want %s", args, err, want)
						}
						if out.Len() != 0 || diagnostic.Len() != 0 {
							t.Fatalf("%v emitted output or a prompt: %q %q", args, out.String(), diagnostic.String())
						}
					}
				}
				if _, err := os.Lstat(".agent-profile"); !os.IsNotExist(err) {
					t.Fatalf("local wrote a dotfile for a missing profile: %v", err)
				}
			})
		}
	}
}
