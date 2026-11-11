package claude_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/linhtruong/agent-profiles/internal/agent/claude"
)

func TestClaudeIdentity(t *testing.T) {
	a := claude.Adapter{}
	for _, tc := range []struct {
		name, got, want string
	}{
		{"Name", a.Name(), "claude"},
		{"EnvVar", a.EnvVar(), "CLAUDE_CONFIG_DIR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("%s() = %q; want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestClaudeDefaultConfigDir(t *testing.T) {
	for _, state := range []string{"set", "empty", "unset"} {
		t.Run(state, func(t *testing.T) {
			home := t.TempDir()
			// Setenv registers restoration even when the test then unsets HOME.
			t.Setenv("HOME", home)
			switch state {
			case "empty":
				t.Setenv("HOME", "")
			case "unset":
				if err := os.Unsetenv("HOME"); err != nil {
					t.Fatal(err)
				}
			}
			dir, err := (claude.Adapter{}).DefaultConfigDir()
			if state != "set" {
				if err == nil || dir != "" {
					t.Fatalf("DefaultConfigDir() with %s HOME = %q, %v; want empty path and error", state, dir, err)
				}
				return
			}
			if want := filepath.Join(home, ".claude"); err != nil || dir != want {
				t.Fatalf("DefaultConfigDir() = %q, %v; want %q", dir, err, want)
			}
		})
	}
}

func TestClaudeInitTemplate(t *testing.T) {
	template := (claude.Adapter{}).InitTemplate()
	if !json.Valid(template) {
		t.Fatalf("InitTemplate() is not valid JSON: %q", template)
	}
}
