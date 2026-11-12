package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// withTokenInput feeds data to --auth-token-stdin through a pipe.
func withTokenInput(t *testing.T, data string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(data); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := tokenInput
	tokenInput = r
	t.Cleanup(func() { tokenInput = old; _ = r.Close() })
}

func TestCreateAuthTokenStdin(t *testing.T) {
	for name, input := range map[string]string{
		"newline": "123456\n",
		"crlf":    "123456\r\n",
		"no eol":  "123456",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
			withTokenInput(t, input)
			var out bytes.Buffer
			args := []string{"create", "claude", "work", "--init", "--base-url", "http://127.0.0.1:8317", "--auth-token-stdin"}
			if err := run(args, &out, &out); err != nil {
				t.Fatal(err)
			}
			settings := readSettings(t, &out)
			env := settings["env"].(map[string]any)
			if env["ANTHROPIC_AUTH_TOKEN"] != "123456" || env["ANTHROPIC_BASE_URL"] != "http://127.0.0.1:8317" {
				t.Fatalf("env = %v", env)
			}
			if _, ok := settings["permissions"]; !ok {
				t.Fatal("--init permissions missing")
			}
		})
	}
}

func TestCreateAuthTokenStdinRejected(t *testing.T) {
	for name, tc := range map[string]struct {
		input string
		args  []string
	}{
		"empty":       {"\n", nil},
		"inner space": {"12 34\n", nil},
		"control":     {"12\x0134\n", nil},
		"too long":    {strings.Repeat("a", maxTokenSize+1) + "\n", nil},
		"duplicate":   {"x\n", []string{"--auth-token-stdin"}},
		"pi":          {"x\n", nil},
	} {
		t.Run(name, func(t *testing.T) {
			data := filepath.Join(t.TempDir(), "data")
			t.Setenv("XDG_DATA_HOME", data)
			withTokenInput(t, tc.input)
			agentName := "claude"
			if name == "pi" {
				agentName = "pi"
			}
			args := append([]string{"create", agentName, "work", "--auth-token-stdin"}, tc.args...)
			var out bytes.Buffer
			if err := run(args, &out, &out); err == nil {
				t.Fatalf("run(%q) succeeded; want error", args)
			}
			if _, err := os.Stat(filepath.Join(data, "agent-profiles", agentName, "work")); !os.IsNotExist(err) {
				t.Fatalf("profile created despite error: %v", err)
			}
			if strings.Contains(out.String(), "12") {
				t.Fatalf("token echoed in output: %q", out.String())
			}
		})
	}
}

func TestCreateAuthTokenStdinExistingProfileFailsBeforeReading(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	withTokenInput(t, "secret\n")
	err := run([]string{"create", "claude", "work", "--auth-token-stdin"}, &out, &out)
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v; want already exists", err)
	}
	rest := make([]byte, 16)
	n, _ := tokenInput.Read(rest)
	if string(rest[:n]) != "secret\n" {
		t.Fatalf("token was consumed before the existence check: %q left", rest[:n])
	}
}
