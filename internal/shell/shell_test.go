package shell

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func roundTrip(t *testing.T, value string) {
	t.Helper()
	code, err := Export("AP_TEST", value)
	if strings.ContainsRune(value, 0) {
		if err == nil {
			t.Fatal("NUL accepted")
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	for _, sh := range []string{"bash", "zsh"} {
		if _, err := exec.LookPath(sh); err != nil {
			t.Skipf("%s unavailable", sh)
		}
		cmd := exec.Command(sh, "-fc", code+"\nprintf '%s' \"$AP_TEST\"")
		got, err := cmd.Output()
		if err != nil || !bytes.Equal(got, []byte(value)) {
			t.Fatalf("%s roundtrip %q: %q %v", sh, value, got, err)
		}
	}
}
func TestNoInjectionViaEmittedShellCode(t *testing.T) {
	for _, v := range []string{"plain", "'\"$`;&\n\r\\", "$(echo PWNED)", "'; echo PWNED; #", string([]byte{255, 254, '\n'}), "end\n", "\x00"} {
		roundTrip(t, v)
	}
	if _, err := Export("X;echo PWNED", "ok"); err == nil {
		t.Fatal("unsafe variable accepted")
	}
}
func FuzzQuote(f *testing.F) {
	for _, v := range []string{"'", "$", "`", ";", "&", "\n", string([]byte{255}), "\x00"} {
		f.Add(v)
	}
	f.Fuzz(func(t *testing.T, v string) {
		if len(v) > 4096 {
			t.Skip()
		}
		roundTrip(t, v)
	})
}
func TestInit(t *testing.T) {
	for _, sh := range []string{"bash", "zsh"} {
		script, err := Init(sh)
		if err != nil || !strings.Contains(script, "ap()") {
			t.Fatalf("%s: %v %q", sh, err, script)
		}
	}
	if _, err := Init("fish"); err == nil {
		t.Fatal("unsupported shell accepted")
	}
}
