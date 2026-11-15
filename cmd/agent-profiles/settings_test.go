package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// nonTerminalStdin points os.Stdin at a regular file for the duration of the test.
func nonTerminalStdin(t *testing.T) {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	old := os.Stdin
	os.Stdin = f
	t.Cleanup(func() { os.Stdin = old })
}

func runOK(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(args, &out, &out); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String()
}

func runErr(t *testing.T, want string, args ...string) {
	t.Helper()
	var out bytes.Buffer
	err := run(args, &out, &out)
	if err == nil {
		t.Fatalf("%v succeeded; want error %q", args, want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("%v error = %q; want it to contain %q", args, err, want)
	}
}

func TestSettingsGetSetRoundTrip(t *testing.T) {
	setupGlobalEnv(t)
	runOK(t, "create", "claude", "work", "--init")
	dir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "agent-profiles", "claude", "work")

	if got := strings.TrimSpace(runOK(t, "settings", "get", "claude", "work", "global-instructions")); got != "on" {
		t.Fatalf("initial get = %q; want on", got)
	}

	got := runOK(t, "settings", "set", "claude", "work", "global-instructions", "off")
	want := "claude/work: global-instructions off (restart running sessions to apply)\n"
	if got != want {
		t.Fatalf("set off output = %q; want %q", got, want)
	}
	if list, _ := excludesIn(t, dir); len(list) != 6 {
		t.Fatalf("excludes after off = %q", list)
	}
	if got := strings.TrimSpace(runOK(t, "settings", "get", "claude", "work", "global-instructions")); got != "off" {
		t.Fatalf("get after off = %q", got)
	}

	if got := runOK(t, "settings", "set", "claude", "work", "global-instructions", "off"); got != "claude/work: global-instructions unchanged\n" {
		t.Fatalf("repeat set output = %q", got)
	}

	runOK(t, "settings", "set", "claude", "work", "global-instructions", "on")
	list, hasFile := excludesIn(t, dir)
	if !hasFile || list != nil {
		t.Fatalf("after on: excludes %q, file %v", list, hasFile)
	}
}

func TestSettingsSetKeepsUserSettings(t *testing.T) {
	setupGlobalEnv(t)
	runOK(t, "create", "claude", "work", "--init")
	dir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "agent-profiles", "claude", "work")
	path := filepath.Join(dir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte(`"allow"`), []byte(`"keepme": 1,
  "allow"`), 1)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	runOK(t, "settings", "set", "claude", "work", "global-instructions", "off")
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"keepme"`, `"statusLine"`, `"defaultMode"`, `"claudeMdExcludes"`} {
		if !strings.Contains(string(got), key) {
			t.Fatalf("settings lost %s:\n%s", key, got)
		}
	}
}

func TestSettingsSetRejectsMalformedExcludesWithoutWriting(t *testing.T) {
	setupGlobalEnv(t)
	runOK(t, "create", "claude", "work", "--init")
	path := filepath.Join(os.Getenv("XDG_DATA_HOME"), "agent-profiles", "claude", "work", "settings.json")
	for name, body := range map[string]string{
		"top-level null": `{"claudeMdExcludes":null,"env":{"A":"b"}}`,
		"null element":   `{"claudeMdExcludes":[null,"/user/**"],"env":{"A":"b"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			runErr(t, "claudeMdExcludes is not a string list", "settings", "set", "claude", "work", "global-instructions", "off")
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != body {
				t.Fatalf("settings.json changed:\n%s", got)
			}
		})
	}
}

func TestSettingsPartialState(t *testing.T) {
	home := setupGlobalEnv(t)
	runOK(t, "create", "claude", "work", "--global-instructions", "off")
	dir := filepath.Join(os.Getenv("XDG_DATA_HOME"), "agent-profiles", "claude", "work")
	path := filepath.Join(dir, "settings.json")
	partial, _ := json.Marshal(map[string][]string{"claudeMdExcludes": {homePaths(home)[0]}})
	if err := os.WriteFile(path, partial, 0o600); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runOK(t, "settings", "get", "claude", "work", "global-instructions")); got != "partial" {
		t.Fatalf("get = %q; want partial", got)
	}
	if got := runOK(t, "settings", "set", "claude", "work", "global-instructions", "on"); !strings.Contains(got, "global-instructions on") {
		t.Fatalf("set on from partial output = %q", got)
	}
}

func TestSettingsLsAndJSON(t *testing.T) {
	setupGlobalEnv(t)
	runOK(t, "create", "claude", "work", "--global-instructions", "off")

	if got := strings.Fields(runOK(t, "settings", "ls", "claude", "work")); !reflect.DeepEqual(got, []string{"global-instructions", "off"}) {
		t.Fatalf("ls = %q", got)
	}
	if got := runOK(t, "settings", "ls", "claude", "work", "--json"); !strings.Contains(got, `"key": "global-instructions"`) || !strings.Contains(got, `"value": "off"`) {
		t.Fatalf("ls --json = %q", got)
	}
	if got := runOK(t, "settings", "get", "claude", "work", "global-instructions", "--json"); !strings.Contains(got, `"value": "off"`) {
		t.Fatalf("get --json = %q", got)
	}
}

func TestSettingsErrors(t *testing.T) {
	setupGlobalEnv(t)
	nonTerminalStdin(t)
	runOK(t, "create", "claude", "work")
	runOK(t, "create", "pi", "work")

	runErr(t, `invalid value "true" for global-instructions: want on or off`, "settings", "set", "claude", "work", "global-instructions", "true")
	runErr(t, `unknown setting "nope". Known settings: global-instructions`, "settings", "get", "claude", "work", "nope")
	runErr(t, `unknown setting "nope". Known settings: global-instructions`, "settings", "set", "claude", "work", "nope", "on")
	runErr(t, `profile "claude/missing" does not exist`, "settings", "ls", "claude", "missing")
	runErr(t, "pi does not support Global instructions yet", "settings", "get", "pi", "work", "global-instructions")
	runErr(t, "pi does not support Global instructions yet", "settings", "set", "pi", "work", "global-instructions", "on")
	runErr(t, "--json works with settings ls and settings get only", "settings", "set", "claude", "work", "global-instructions", "on", "--json")
	runErr(t, "unknown option", "settings", "--bogus")
	runErr(t, "usage: ap settings", "settings", "get", "claude", "work")
	runErr(t, "usage: ap settings", "settings", "set", "claude", "work", "global-instructions")
	runErr(t, "usage: ap settings", "settings", "ls", "claude")
	runErr(t, "ap settings needs a terminal", "settings")
	runErr(t, "ap settings needs a terminal", "settings", "claude", "work")
}

func TestSettingsBareNeedsTerminal(t *testing.T) {
	setupGlobalEnv(t)
	nonTerminalStdin(t)
	runErr(t, "use ap settings get or ap settings set", "settings")
}

func TestSettingsPiListsNothing(t *testing.T) {
	setupGlobalEnv(t)
	runOK(t, "create", "pi", "work")
	if got := runOK(t, "settings", "ls", "pi", "work"); got != "no Profile settings for pi\n" {
		t.Fatalf("ls pi = %q", got)
	}
	if got := runOK(t, "settings", "ls", "pi", "work", "--json"); strings.TrimSpace(got) != "[]" {
		t.Fatalf("ls pi --json = %q", got)
	}
}
