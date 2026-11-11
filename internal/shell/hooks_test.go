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
