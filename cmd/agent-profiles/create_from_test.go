package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func createWithSettings(t *testing.T, name string, settings []byte) string {
	t.Helper()
	var out bytes.Buffer
	if err := run([]string{"create", "claude", name}, &out, &out); err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSpace(out.String())
	if settings != nil {
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), settings, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestCreateFrom(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	src := []byte("{\"env\":{\"ANTHROPIC_BASE_URL\":\"http://a\",\"ANTHROPIC_AUTH_TOKEN\":\"t\"}}\n")
	srcDir := createWithSettings(t, "src", src)
	// Account state and runtime data must not be copied.
	for _, f := range []string{".claude.json", "history.jsonl"} {
		if err := os.WriteFile(filepath.Join(srcDir, f), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var out bytes.Buffer
	if err := run([]string{"create", "claude", "dst", "--from", "src"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	dst := strings.TrimSpace(out.String())
	got, err := os.ReadFile(filepath.Join(dst, "settings.json"))
	if err != nil || !bytes.Equal(got, src) {
		t.Fatalf("copied settings = %q, %v; want %q", got, err, src)
	}
	if st, err := os.Stat(filepath.Join(dst, "settings.json")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("copied settings mode = %v, %v; want 0600", st.Mode().Perm(), err)
	}
	entries, err := os.ReadDir(dst)
	if err != nil || len(entries) != 1 {
		t.Fatalf("dst contains %v, %v; want only settings.json", entries, err)
	}

	out.Reset()
	if err := run([]string{"create", "claude", "dst2", "--from", "src", "--base-url", "http://b"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	settings := readSettings(t, &out)
	env := settings["env"].(map[string]any)
	if env["ANTHROPIC_BASE_URL"] != "http://b" || env["ANTHROPIC_AUTH_TOKEN"] != "t" {
		t.Fatalf("env = %v; want base URL overridden and token kept", env)
	}
}

func TestCreateFromWithoutSettings(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	createWithSettings(t, "empty", nil)
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "dst", "--from", "empty"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(strings.TrimSpace(out.String()), "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("settings.json created from empty source: %v", err)
	}
}

func TestCreateFromRejected(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	createWithSettings(t, "src", []byte("{}\n"))
	linkDir := createWithSettings(t, "link", nil)
	outside := filepath.Join(t.TempDir(), "secret.json")
	if err := os.WriteFile(outside, []byte("{\"secret\":1}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(linkDir, "settings.json")); err != nil {
		t.Fatal(err)
	}
	createWithSettings(t, "big", bytes.Repeat([]byte(" "), 1<<20+1))

	for name, args := range map[string][]string{
		"with init":     {"create", "claude", "dst", "--init", "--from", "src"},
		"missing value": {"create", "claude", "dst", "--from"},
		"self":          {"create", "claude", "src", "--from", "src"},
		"missing src":   {"create", "claude", "dst", "--from", "nope"},
		"bad name":      {"create", "claude", "dst", "--from", "../src"},
		"symlink":       {"create", "claude", "dst", "--from", "link"},
		"too large":     {"create", "claude", "dst", "--from", "big"},
		"duplicate":     {"create", "claude", "dst", "--from", "src", "--from", "src"},
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := run(args, &out, &out); err == nil {
				t.Fatalf("run(%q) succeeded; want error", args)
			}
			var list bytes.Buffer
			if err := run([]string{"list", "claude"}, &list, &list); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(list.String(), "dst") {
				t.Fatalf("profile created despite error: %q", list.String())
			}
		})
	}
}
