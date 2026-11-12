package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/agent/claude"
)

func templateStatusLine(t *testing.T) json.RawMessage {
	t.Helper()
	var tmpl map[string]json.RawMessage
	if err := json.Unmarshal((claude.Adapter{}).InitTemplate(), &tmpl); err != nil {
		t.Fatal(err)
	}
	return tmpl["statusLine"]
}

func TestUpdateRefreshesStatusLineAndKeepsUserSettings(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	old := []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"secret"},"permissions":{"allow":["Read"]},"statusLine":{"type":"command","command":"old"}}`)
	dir := createWithSettings(t, "work", old)
	if err := os.Chmod(filepath.Join(dir, "settings.json"), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := run([]string{"update", "claude", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "claude/work: updated\n" {
		t.Fatalf("output = %q", got)
	}
	path := filepath.Join(dir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if !jsonEqual(t, got["statusLine"], templateStatusLine(t)) {
		t.Fatalf("statusLine = %s", got["statusLine"])
	}
	if !jsonEqual(t, got["env"], json.RawMessage(`{"ANTHROPIC_AUTH_TOKEN":"secret"}`)) {
		t.Fatalf("env changed: %s", got["env"])
	}
	if !jsonEqual(t, got["permissions"], json.RawMessage(`{"allow":["Read"]}`)) {
		t.Fatalf("permissions changed: %s", got["permissions"])
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v; want 0600", st.Mode().Perm())
	}
	if _, err := os.Lstat(filepath.Join(dir, ".settings.json.tmp")); !os.IsNotExist(err) {
		t.Fatalf("temp file left behind: %v", err)
	}

	out.Reset()
	if err := run([]string{"update", "claude", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "claude/work: unchanged\n" {
		t.Fatalf("second run output = %q", got)
	}
}

func TestUpdateAll(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	createWithSettings(t, "a", []byte(`{}`))
	noSettings := createWithSettings(t, "b", nil)

	var out bytes.Buffer
	if err := run([]string{"update", "claude", "--all"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); got != "claude/a: updated\nclaude/b: updated\n" {
		t.Fatalf("output = %q", got)
	}
	if _, err := os.Stat(filepath.Join(noSettings, "settings.json")); err != nil {
		t.Fatalf("settings.json not created for profile without one: %v", err)
	}
}

func TestUpdateRejects(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	dir := createWithSettings(t, "bad", []byte(`not json`))
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"usage", []string{"update", "claude"}, "usage"},
		{"missing profile", []string{"update", "claude", "nope"}, "does not exist"},
		{"invalid json", []string{"update", "claude", "bad"}, "parse settings"},
		{"unsupported agent", []string{"update", "pi", "x"}, "no settings to update"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(tc.args, &out, &out)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v; want %q", err, tc.want)
			}
		})
	}
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil || string(data) != "not json" {
		t.Fatalf("invalid settings were modified: %q %v", data, err)
	}
}

func jsonEqual(t *testing.T, a, b json.RawMessage) bool {
	t.Helper()
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &y); err != nil {
		t.Fatal(err)
	}
	xs, _ := json.Marshal(x)
	ys, _ := json.Marshal(y)
	return bytes.Equal(xs, ys)
}
