package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoSwitchRequiresPerDirectoryTrust(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	dir := filepath.Join(home, "repo")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	call := func(args ...string) (string, string, error) {
		var out, diag bytes.Buffer
		err := run(args, &out, &diag)
		return out.String(), diag.String(), err
	}
	for _, args := range [][]string{{"create", "claude", "work"}, {"create", "claude", "personal"}, {"local", "claude", "work"}} {
		if _, _, err := call(args...); err != nil {
			t.Fatal(err)
		}
	}
	out, diag, err := call("auto", "--emit-shell")
	if err != nil || out != "" || !strings.Contains(diag, "trust") || strings.Count(diag, "\n") != 1 {
		t.Fatalf("untrusted auto: %q %q %v", out, diag, err)
	}
	out, _, err = call("current")
	if err != nil || !strings.Contains(out, "default") {
		t.Fatalf("untrusted current: %q %v", out, err)
	}
	if _, _, err = call("trust"); err != nil {
		t.Fatal(err)
	}
	out, diag, err = call("auto", "--emit-shell")
	if err != nil || !strings.Contains(out, "export CLAUDE_CONFIG_DIR=") || diag != "" {
		t.Fatalf("trusted auto: %q %q %v", out, diag, err)
	}
	// The env var is unset here, so current reports the Default config dir even though the Dotfile is trusted.
	out, _, err = call("current")
	if err != nil || !strings.Contains(out, "(default)") || strings.Contains(out, "dotfile") {
		t.Fatalf("trusted current with env unset: %q %v", out, err)
	}
	if _, _, err = call("local", "claude", "personal"); err != nil {
		t.Fatal(err)
	}
	out, diag, err = call("auto", "--emit-shell")
	if err != nil || out != "" || !strings.Contains(diag, "trust") {
		t.Fatalf("changed auto: %q %q %v", out, diag, err)
	}
	if _, _, err = call("trust"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = call("untrust"); err != nil {
		t.Fatal(err)
	}
	out, _, err = call("auto", "--emit-shell")
	if err != nil || out != "" {
		t.Fatalf("revoked auto: %q %v", out, err)
	}
}
func TestAutoHookIntegration(t *testing.T) {
	cache, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOMODCACHE", strings.TrimSpace(string(cache)))
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	bin := filepath.Join(t.TempDir(), "agent-profiles")
	if got, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build %s %v", got, err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, sh := range []string{"bash", "zsh"} {
		if _, err := exec.LookPath(sh); err != nil {
			t.Skipf("%s unavailable", sh)
		}
		script := `mkdir -p "$HOME/` + sh + `/repo"
cd "$HOME/` + sh + `" || exit 1
agent-profiles create claude ` + sh + ` >/dev/null || exit 2
cd repo || exit 3
agent-profiles local claude ` + sh + ` || exit 4
eval "$(agent-profiles init ` + sh + `)"
printf '%s\000' "${CLAUDE_CONFIG_DIR-unset}"
agent-profiles trust || exit 5
` + func() string {
			if sh == "bash" {
				return `PROMPT_COMMAND='printf prior >&2'
eval "$(agent-profiles init bash)"
_ap_auto
`
			}
			return `cd ..
cd repo
`
		}() + `printf '%s\000' "$CLAUDE_CONFIG_DIR"
agent-profiles untrust || exit 6
unset CLAUDE_CONFIG_DIR
_ap_auto
printf '%s' "${CLAUDE_CONFIG_DIR-unset}"
`
		cmd := exec.Command(sh, "-fc", script)
		got, err := cmd.Output()
		if err != nil {
			t.Fatalf("%s: %q %v", sh, got, err)
		}
		parts := strings.Split(string(got), "\x00")
		if len(parts) != 3 || parts[0] != "" || !strings.HasSuffix(parts[1], "/claude/"+sh) || parts[2] != "unset" {
			t.Fatalf("%s hook: %q", sh, got)
		}
	}
}
