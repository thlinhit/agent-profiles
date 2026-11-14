package shell

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestHooksPreserveExistingHooksAndStatus(t *testing.T) {
	dir := t.TempDir()
	stub := filepath.Join(dir, "agent-profiles")
	if err := os.WriteFile(stub, []byte("#!/bin/sh\nexit 0\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	for _, sh := range []string{"bash", "zsh"} {
		if _, err := exec.LookPath(sh); err != nil {
			t.Skipf("%s unavailable", sh)
		}
		init, err := Init(sh)
		if err != nil {
			t.Fatal(err)
		}
		if sh == "zsh" && strings.Contains(init, "_ap_last_pwd") {
			t.Fatal("Bash prompt guard leaked into zsh")
		}
		prefix := ""
		assertions := ""
		if sh == "bash" {
			prefix = "PROMPT_COMMAND='printf prior'\n"
			assertions = `[[ "$PROMPT_COMMAND" == '_ap_auto; printf prior' ]] || exit 2
false
_ap_auto
[[ $? == 1 ]] || exit 3
PROMPT_COMMAND=('printf first' 'printf second')
` + init + init + `[[ ${#PROMPT_COMMAND[@]} == 3 ]] || exit 4
[[ ${PROMPT_COMMAND[0]} == _ap_auto ]] || exit 5
[[ ${PROMPT_COMMAND[1]} == 'printf first' ]] || exit 6
[[ ${PROMPT_COMMAND[2]} == 'printf second' ]] || exit 7
`
		} else {
			prefix = "prior() { :; }\nchpwd_functions=(prior)\n"
			assertions = `[[ ${#chpwd_functions[@]} == 2 ]] || exit 2
[[ ${chpwd_functions[1]} == prior && ${chpwd_functions[2]} == _ap_auto ]] || exit 3
false
_ap_auto
[[ $? == 1 ]] || exit 4
`
		}
		got, err := exec.Command(sh, "-fc", prefix+init+init+assertions).CombinedOutput()
		if err != nil || strings.TrimSpace(string(got)) != "" {
			t.Fatalf("%s: %s %v", sh, got, err)
		}
	}
}

func TestBashPromptGuard(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash unavailable")
	}
	dir := t.TempDir()
	stub := `#!/bin/sh
case "$1" in
  auto)
    printf x >> "$AP_AUTO_LOG"
    [ "${AP_FAIL_AUTO-}" != 1 ] || exit 9
    ;;
  fail) exit 7 ;;
esac
`
	if err := os.WriteFile(filepath.Join(dir, "agent-profiles"), []byte(stub), 0o700); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other ' [$]")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("AP_AUTO_LOG", filepath.Join(dir, "calls"))
	t.Setenv("AP_OTHER_DIR", other)
	init, err := Init("bash")
	if err != nil {
		t.Fatal(err)
	}
	script := `expect_calls() {
  local calls
  calls=$(wc -c < "$AP_AUTO_LOG")
  [[ $calls -eq $1 ]] || { printf 'calls=%s want=%s\n' "$calls" "$1"; exit 1; }
}
_ap_last_pwd=$PWD
` + init + `expect_calls 1
false
_ap_auto
[[ $? == 1 ]] || exit 2
expect_calls 1
_ap_start_pwd=$PWD
cd "$AP_OTHER_DIR" || exit 3
(exit 37)
_ap_auto
[[ $? == 37 ]] || exit 4
expect_calls 2
_ap_auto
expect_calls 2
export AP_FAIL_AUTO=1
cd "$_ap_start_pwd" || exit 5
(exit 23)
_ap_auto
[[ $? == 23 ]] || exit 6
expect_calls 3
_ap_auto
expect_calls 3
unset AP_FAIL_AUTO
ap auto || exit 7
expect_calls 4
eval "$(agent-profiles auto --emit-shell)"
expect_calls 5
ap trust || exit 8
_ap_auto
expect_calls 6
ap untrust || exit 9
_ap_auto
expect_calls 7
ap local || exit 10
_ap_auto
expect_calls 8
ap fail
[[ $? == 7 ]] || exit 11
_ap_auto
expect_calls 8
` + init + `expect_calls 9
`
	if got, err := exec.Command("bash", "-fc", script).CombinedOutput(); err != nil {
		t.Fatalf("Bash guard: %s %v", got, err)
	}
}
