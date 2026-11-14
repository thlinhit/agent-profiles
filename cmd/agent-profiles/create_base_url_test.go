package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readSettings(t *testing.T, out *bytes.Buffer) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(out.String()), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("settings.json is not valid JSON: %v", err)
	}
	return settings
}

func TestCreateBaseURL(t *testing.T) {
	const u = "http://127.0.0.1:8317"
	for _, args := range [][]string{
		{"create", "claude", "work", "--base-url", u},
		{"create", "claude", "work", "--init", "--base-url", u},
		{"create", "claude", "work", "--base-url", u, "--init"},
	} {
		t.Run(strings.Join(args[3:], " "), func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
			var out bytes.Buffer
			if err := run(args, &out, &out); err != nil {
				t.Fatal(err)
			}
			settings := readSettings(t, &out)
			env, _ := settings["env"].(map[string]any)
			if env["ANTHROPIC_BASE_URL"] != u {
				t.Fatalf("env.ANTHROPIC_BASE_URL = %v; want %s", env["ANTHROPIC_BASE_URL"], u)
			}
			_, hasPermissions := settings["permissions"]
			wantPermissions := strings.Contains(strings.Join(args, " "), "--init")
			if hasPermissions != wantPermissions {
				t.Fatalf("permissions present = %v; want %v", hasPermissions, wantPermissions)
			}
		})
	}
}

func TestCreateBaseURLFlagContext(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	var out bytes.Buffer
	err := run([]string{"create", "claude", "work", "--base-url", "file:///etc/passwd"}, &out, &out)
	want := `--base-url: invalid base URL "file:///etc/passwd": want http(s)://host[:port][/path]`
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v; want %q", err, want)
	}
}

func TestCreateBaseURLRejected(t *testing.T) {
	for name, args := range map[string][]string{
		"missing value":     {"create", "claude", "work", "--base-url"},
		"empty value":       {"create", "claude", "work", "--base-url", ""},
		"duplicate":         {"create", "claude", "work", "--base-url", "http://a", "--base-url", "http://b"},
		"duplicate init":    {"create", "claude", "work", "--init", "--init"},
		"unknown option":    {"create", "claude", "work", "--yes"},
		"bad scheme":        {"create", "claude", "work", "--base-url", "file:///etc/passwd"},
		"no host":           {"create", "claude", "work", "--base-url", "http://"},
		"credentials":       {"create", "claude", "work", "--base-url", "http://user:token@127.0.0.1:8317"},
		"query":             {"create", "claude", "work", "--base-url", "http://127.0.0.1:8317?key=x"},
		"unsupported agent": {"create", "pi", "work", "--base-url", "http://127.0.0.1:8317"},
	} {
		t.Run(name, func(t *testing.T) {
			data := filepath.Join(t.TempDir(), "data")
			t.Setenv("XDG_DATA_HOME", data)
			var out bytes.Buffer
			if err := run(args, &out, &out); err == nil {
				t.Fatalf("run(%q) succeeded; want error", args)
			}
			if _, err := os.Stat(filepath.Join(data, "agent-profiles", args[1], "work")); !os.IsNotExist(err) {
				t.Fatalf("profile created despite error: %v", err)
			}
		})
	}
}
