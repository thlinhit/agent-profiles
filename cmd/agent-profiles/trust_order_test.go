package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/dotfile"
)

func TestUntrustedDotfileIsNeverParsed(t *testing.T) {
	for _, content := range []string{
		"0=00:00:00", // testdata/fuzz/FuzzParse/3ef010c93dd77104: former go-toml panic.
		"claude = [", // Syntax error, not merely a schema mismatch.
	} {
		for _, previouslyTrusted := range []bool{false, true} {
			t.Run(fmtTestName(content, previouslyTrusted), func(t *testing.T) {
				home := t.TempDir()
				t.Setenv("HOME", home)
				t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
				t.Setenv("CLAUDE_CONFIG_DIR", "")
				t.Chdir(home)
				path := filepath.Join(home, dotfile.Filename)
				call := func(args ...string) (string, string, error) {
					var out, diag bytes.Buffer
					err := run(args, &out, &diag)
					return out.String(), diag.String(), err
				}
				if previouslyTrusted {
					if err := os.WriteFile(path, []byte("claude='work'"), 0o600); err != nil {
						t.Fatal(err)
					}
					if _, _, err := call("trust"); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				if _, err := dotfile.Parse([]byte(content)); err == nil {
					t.Fatal("test input must fail if parsed")
				}
				for _, command := range []string{"auto", "current"} {
					out, diag, err := call(command)
					if err != nil {
						t.Fatalf("%s parsed untrusted content: %v", command, err)
					}
					if !strings.Contains(diag, "untrusted dotfile") || strings.Count(diag, "\n") != 1 {
						t.Fatalf("%s hint: %q", command, diag)
					}
					if command == "auto" && out != "" {
						t.Fatalf("auto emitted code: %q", out)
					}
					if command == "current" && !strings.Contains(out, "(default)") {
						t.Fatalf("current selected untrusted profile: %q", out)
					}
				}
				// Explicit approval records raw bytes; only subsequent resolution parses them.
				if _, _, err := call("trust"); err != nil {
					t.Fatalf("raw-byte approval parsed content: %v", err)
				}
				for _, command := range []string{"auto", "current"} {
					out, _, err := call(command)
					if err == nil || out != "" {
						t.Fatalf("%s failed to parse after trust: %q %v", command, out, err)
					}
				}
			})
		}
	}
}

func fmtTestName(content string, previouslyTrusted bool) string {
	name := "malformed"
	if content == "0=00:00:00" {
		name = "known_crasher"
	}
	if previouslyTrusted {
		return name + "/changed_hash"
	}
	return name + "/never_trusted"
}
