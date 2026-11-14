package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// setupTrustedSelection creates claude/work, writes a Dotfile selecting it in the
// working directory, and trusts that Dotfile. It leaves CLAUDE_CONFIG_DIR unset.
func setupTrustedSelection(t *testing.T) (profilePath string, call func(args ...string) (string, string, error)) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	dir := filepath.Join(home, "repo")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	call = func(args ...string) (string, string, error) {
		var out, diag bytes.Buffer
		err := run(args, &out, &diag)
		return out.String(), diag.String(), err
	}
	for _, args := range [][]string{{"create", "claude", "work"}, {"local", "claude", "work"}, {"trust"}} {
		if _, _, err := call(args...); err != nil {
			t.Fatal(err)
		}
	}
	profilePath, err := (profile.Store{}).Resolve("claude", "work")
	if err != nil {
		t.Fatal(err)
	}
	return profilePath, call
}

func TestCurrentReportsSourceFromEnvironment(t *testing.T) {
	profilePath, call := setupTrustedSelection(t)
	a, err := agent.Get("claude")
	if err != nil {
		t.Fatal(err)
	}
	defaultDir, err := a.DefaultConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, env, path, source string
	}{
		{"unset ignores trusted dotfile", "", defaultDir, "default"},
		{"equal to selection", profilePath, profilePath, "dotfile"},
		{"equal after Clean", profilePath + string(filepath.Separator) + ".", profilePath + string(filepath.Separator) + ".", "dotfile"},
		{"set to other path", "/custom/claude", "/custom/claude", "env"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("CLAUDE_CONFIG_DIR", tc.env)

			out, _, err := call("current")
			if err != nil {
				t.Fatal(err)
			}
			want := "claude " + tc.path + " (" + tc.source + ")"
			if !containsLine(out, want) {
				t.Errorf("text: want line %q in %q", want, out)
			}

			out, _, err = call("current", "--json")
			if err != nil {
				t.Fatal(err)
			}
			var items []struct {
				Agent, Path, Source string
			}
			if err := json.Unmarshal([]byte(out), &items); err != nil {
				t.Fatalf("json %q: %v", out, err)
			}
			for _, item := range items {
				if item.Agent == "claude" && (item.Path != tc.path || item.Source != tc.source) {
					t.Errorf("json: got path=%q source=%q, want path=%q source=%q", item.Path, item.Source, tc.path, tc.source)
				}
			}
		})
	}
}

func containsLine(out, want string) bool {
	for _, line := range strings.Split(out, "\n") {
		if line == want {
			return true
		}
	}
	return false
}
