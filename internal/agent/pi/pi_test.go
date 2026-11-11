package pi_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/linhtruong/agent-profiles/internal/agent/pi"
)

func TestPiIdentity(t *testing.T) {
	a := pi.Adapter{}
	for _, tc := range []struct {
		name, got, want string
	}{
		{"Name", a.Name(), "pi"},
		{"EnvVar", a.EnvVar(), "PI_CODING_AGENT_DIR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("%s() = %q; want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestPiDefaultConfigDir(t *testing.T) {
	for _, state := range []string{"set", "empty", "unset"} {
		t.Run(state, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			// The default is independent of a currently selected profile.
			t.Setenv("PI_CODING_AGENT_DIR", filepath.Join(home, "other-profile"))
			switch state {
			case "empty":
				t.Setenv("HOME", "")
			case "unset":
				if err := os.Unsetenv("HOME"); err != nil {
					t.Fatal(err)
				}
			}
			dir, err := (pi.Adapter{}).DefaultConfigDir()
			if state != "set" {
				if err == nil || dir != "" {
					t.Fatalf("DefaultConfigDir() with %s HOME = %q, %v; want empty path and error", state, dir, err)
				}
				return
			}
			if want := filepath.Join(home, ".pi", "agent"); err != nil || dir != want {
				t.Fatalf("DefaultConfigDir() = %q, %v; want %q", dir, err, want)
			}
		})
	}
}

func TestPiInitTemplate(t *testing.T) {
	template := (pi.Adapter{}).InitTemplate()
	if !json.Valid(template) {
		t.Fatalf("InitTemplate() is not valid JSON: %q", template)
	}
}
