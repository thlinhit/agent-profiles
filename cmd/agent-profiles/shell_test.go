package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellSwitchIntegration(t *testing.T) {
	cache, err := exec.Command("go", "env", "GOMODCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GOMODCACHE", strings.TrimSpace(string(cache)))
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data ' $`;&\n"))
	bin := filepath.Join(t.TempDir(), "agent-profiles")
	build := exec.Command("go", "build", "-o", bin, ".")
	if got, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", got, err)
	}
	t.Setenv("PATH", filepath.Dir(bin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	want := strings.TrimSuffix(out.String(), "\n")
	for _, sh := range []string{"bash", "zsh"} {
		if _, err := exec.LookPath(sh); err != nil {
			t.Skipf("%s unavailable", sh)
		}
		script := `eval "$(agent-profiles init ` + sh + `)"
ap use claude work || exit 2
printf '%s\000' "$CLAUDE_CONFIG_DIR"
ap current || exit 3
ap list claude || exit 4
ap use claude invalid/name >/dev/null 2>&1 && exit 5
printf '%s\000' "$CLAUDE_CONFIG_DIR"
ap off claude || exit 6
printf '%s' "${CLAUDE_CONFIG_DIR-unset}"
`
		if sh == "bash" {
			script += `
mkdir "$HOME/repo" || exit 10
cd "$HOME/repo" || exit 11
unset CLAUDE_CONFIG_DIR
ap local claude work || exit 12
_ap_auto
[[ ${CLAUDE_CONFIG_DIR-unset} == unset ]] || exit 13
ap trust || exit 14
_ap_auto
[[ $CLAUDE_CONFIG_DIR == *'/claude/work' ]] || exit 15
unset CLAUDE_CONFIG_DIR
false
_ap_auto
[[ $? == 1 && ${CLAUDE_CONFIG_DIR-unset} == unset ]] || exit 16
ap auto || exit 17
[[ $CLAUDE_CONFIG_DIR == *'/claude/work' ]] || exit 18
unset CLAUDE_CONFIG_DIR
eval "$(agent-profiles auto --emit-shell)"
[[ $CLAUDE_CONFIG_DIR == *'/claude/work' ]] || exit 19
ap untrust || exit 20
unset CLAUDE_CONFIG_DIR
_ap_auto
[[ ${CLAUDE_CONFIG_DIR-unset} == unset ]] || exit 21
agent-profiles trust || exit 22
ap auto || exit 23
[[ $CLAUDE_CONFIG_DIR == *'/claude/work' ]] || exit 24
cd "$HOME" || exit 25
unset CLAUDE_CONFIG_DIR
_ap_auto
cd repo || exit 26
_ap_auto
[[ $CLAUDE_CONFIG_DIR == *'/claude/work' ]] || exit 27
`
		}
		cmd := exec.Command(sh, "-fc", script)
		var diag bytes.Buffer
		cmd.Stderr = &diag
		got, err := cmd.Output()
		expected := want + "\x00" + want + "\x00unset"
		if err != nil || string(got) != expected {
			t.Fatalf("%s: got %q want %q err %v diagnostics %s", sh, got, expected, err, &diag)
		}
	}
}
func TestCurrentAndEmitOutput(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_CONFIG_DIR", "/custom")
	var out, diag bytes.Buffer
	if err := run([]string{"current"}, &out, &diag); err != nil || !strings.Contains(out.String(), "env") {
		t.Fatalf("current: %s %v", &out, err)
	}
	out.Reset()
	if err := run([]string{"current", "--emit-shell"}, &out, &diag); err != nil || out.Len() != 0 {
		t.Fatalf("human text on eval channel: %q %v", out.String(), err)
	}
}
