package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// TestMain keeps every test non-interactive, even when go test runs in a terminal.
func TestMain(m *testing.M) {
	stdinIsTerminal = func() bool { return false }
	stderrIsTerminal = func() bool { return false }
	os.Exit(m.Run())
}

// setupGlobalEnv isolates Profile storage and HOME, so the six home paths are predictable.
func setupGlobalEnv(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

// withTerminal makes create think stdin and stderr are terminals and answers the question with ask.
func withTerminal(t *testing.T, ask func() (bool, error)) {
	t.Helper()
	withTerminals(t, true, true, ask)
}

// withTerminals sets what create sees for stdin and stderr and answers the question with ask.
func withTerminals(t *testing.T, stdin, stderr bool, ask func() (bool, error)) {
	t.Helper()
	oldIn, oldErr, oldAsk := stdinIsTerminal, stderrIsTerminal, askGlobalInstructions
	stdinIsTerminal = func() bool { return stdin }
	stderrIsTerminal = func() bool { return stderr }
	askGlobalInstructions = ask
	t.Cleanup(func() { stdinIsTerminal, stderrIsTerminal, askGlobalInstructions = oldIn, oldErr, oldAsk })
}

func failAsk(t *testing.T) func() (bool, error) {
	return func() (bool, error) {
		t.Fatal("question asked; it must not be")
		return false, nil
	}
}

func homePaths(home string) []string {
	return []string{
		filepath.Join(home, ".claude", "CLAUDE.md"),
		filepath.Join(home, ".claude", "rules", "**"),
		filepath.Join(home, "AGENTS.md"),
		filepath.Join(home, ".claude", "AGENTS.md"),
		filepath.Join(home, "CLAUDE.md"),
		filepath.Join(home, "CLAUDE.local.md"),
	}
}

// excludesIn returns claudeMdExcludes of a Profile's settings.json, and whether the file exists.
func excludesIn(t *testing.T, dir string) ([]string, bool) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if os.IsNotExist(err) {
		return nil, false
	}
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	raw, ok := m["claudeMdExcludes"]
	if !ok {
		return nil, true
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	return list, true
}

func stateOf(t *testing.T, agentName, name string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run([]string{"settings", "get", agentName, name, "global-instructions"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

func TestCreateGlobalInstructionsFlag(t *testing.T) {
	home := setupGlobalEnv(t)
	withTerminal(t, failAsk(t)) // the flag must suppress the question
	tests := []struct {
		name      string
		args      []string
		wantState string
		wantFile  bool
	}{
		{"on writes nothing", []string{"--global-instructions", "on"}, "on", false},
		{"off writes excludes", []string{"--global-instructions", "off"}, "off", true},
		{"off with init keeps template", []string{"--init", "--global-instructions", "off"}, "off", true},
		{"on with init keeps template", []string{"--init", "--global-instructions", "on"}, "on", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			name := strings.ReplaceAll(tc.name, " ", "-")
			var out bytes.Buffer
			if err := run(append([]string{"create", "claude", name}, tc.args...), &out, &out); err != nil {
				t.Fatal(err)
			}
			dir := strings.TrimSpace(out.String())
			list, hasFile := excludesIn(t, dir)
			if hasFile != tc.wantFile {
				t.Fatalf("settings.json present = %v; want %v", hasFile, tc.wantFile)
			}
			if tc.wantState == "off" && !reflect.DeepEqual(list, homePaths(home)) {
				t.Fatalf("excludes = %q; want home paths", list)
			}
			if got := stateOf(t, "claude", name); got != tc.wantState {
				t.Fatalf("state = %q; want %q", got, tc.wantState)
			}
		})
	}
}

func TestCreateGlobalInstructionsInitKeepsPermissions(t *testing.T) {
	setupGlobalEnv(t)
	withTerminal(t, failAsk(t))
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "work", "--init", "--global-instructions", "off"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(strings.TrimSpace(out.String()), "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{`"permissions"`, `"statusLine"`, `"claudeMdExcludes"`, `"attribution"`} {
		if !strings.Contains(string(data), key) {
			t.Fatalf("settings missing %s:\n%s", key, data)
		}
	}
}

func TestCreateGlobalInstructionsRejected(t *testing.T) {
	setupGlobalEnv(t)
	withTerminal(t, failAsk(t))
	tests := map[string]struct {
		args []string
		want string
	}{
		"bad value":   {[]string{"--global-instructions", "true"}, `--global-instructions must be on or off, not "true"`},
		"empty value": {[]string{"--global-instructions", ""}, "--global-instructions requires on or off"},
		"missing":     {[]string{"--global-instructions"}, "--global-instructions requires on or off"},
		"duplicate":   {[]string{"--global-instructions", "on", "--global-instructions", "off"}, `duplicate option "--global-instructions"`},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(append([]string{"create", "claude", "bad"}, tc.args...), &out, &out)
			if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v; want %q", err, tc.want)
			}
			assertNoProfile(t, "bad")
		})
	}
}

func assertNoProfile(t *testing.T, name string) {
	t.Helper()
	var list bytes.Buffer
	if err := run([]string{"list"}, &list, &list); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(list.String(), name) {
		t.Fatalf("profile %q created despite error: %q", name, list.String())
	}
}

func TestCreateGlobalInstructionsPiRejected(t *testing.T) {
	setupGlobalEnv(t)
	withTerminal(t, failAsk(t))
	var out bytes.Buffer
	err := run([]string{"create", "pi", "work", "--global-instructions", "off"}, &out, &out)
	if err == nil || err.Error() != "pi does not support Global instructions yet" {
		t.Fatalf("error = %v", err)
	}
	assertNoProfile(t, "work")
}

func TestCreateGlobalInstructionsPiNeverAsks(t *testing.T) {
	setupGlobalEnv(t)
	withTerminal(t, failAsk(t))
	var out bytes.Buffer
	if err := run([]string{"create", "pi", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
}

func TestCreateGlobalInstructionsFromKeepsState(t *testing.T) {
	home := setupGlobalEnv(t)
	withTerminal(t, failAsk(t))
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "src", "--global-instructions", "off"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := run([]string{"create", "claude", "dst", "--from", "src"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, "claude", "dst"); got != "off" {
		t.Fatalf("--from state = %q; want off", got)
	}
	out.Reset()
	if err := run([]string{"create", "claude", "flagged", "--from", "src", "--global-instructions", "on"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if got := stateOf(t, "claude", "flagged"); got != "on" {
		t.Fatalf("flag over --from state = %q; want on", got)
	}
	if list, _ := excludesIn(t, strings.TrimSpace(out.String())); list != nil {
		t.Fatalf("excludes left after on: %q (home %s)", list, home)
	}
}

func TestCreateGlobalInstructionsAsk(t *testing.T) {
	setupGlobalEnv(t)
	tests := []struct {
		name      string
		answer    bool
		wantState string
	}{
		{"yes", true, "on"},
		{"no", false, "off"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			asked := 0
			withTerminal(t, func() (bool, error) { asked++; return tc.answer, nil })
			var out bytes.Buffer
			if err := run([]string{"create", "claude", tc.name}, &out, &out); err != nil {
				t.Fatal(err)
			}
			if asked != 1 {
				t.Fatalf("asked %d times; want 1", asked)
			}
			if got := stateOf(t, "claude", tc.name); got != tc.wantState {
				t.Fatalf("state = %q; want %q", got, tc.wantState)
			}
		})
	}
}

func TestCreateGlobalInstructionsAskSkippedWithoutTerminal(t *testing.T) {
	setupGlobalEnv(t)
	oldAsk := askGlobalInstructions
	askGlobalInstructions = failAsk(t)
	t.Cleanup(func() { askGlobalInstructions = oldAsk })
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "piped"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSpace(out.String())
	if _, hasFile := excludesIn(t, dir); hasFile {
		t.Fatal("settings.json written without a terminal answer")
	}
	if got := stateOf(t, "claude", "piped"); got != "on" {
		t.Fatalf("state = %q; want on", got)
	}
}

func TestCreateGlobalInstructionsNoQuestionWithoutStderrTerminal(t *testing.T) {
	setupGlobalEnv(t)
	withTerminals(t, true, false, failAsk(t)) // stdin is a terminal but the question would be hidden
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "hidden"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if _, hasFile := excludesIn(t, strings.TrimSpace(out.String())); hasFile {
		t.Fatal("settings.json written without an answer")
	}
	if got := stateOf(t, "claude", "hidden"); got != "on" {
		t.Fatalf("state = %q; want on", got)
	}
}

func TestCreateGlobalInstructionsCancelled(t *testing.T) {
	setupGlobalEnv(t)
	withTerminal(t, func() (bool, error) { return false, errCancelled })
	var out bytes.Buffer
	err := run([]string{"create", "claude", "nope"}, &out, &out)
	if !errors.Is(err, errCancelled) {
		t.Fatalf("error = %v; want cancelled", err)
	}
	var list bytes.Buffer
	if err := run([]string{"list", "claude"}, &list, &list); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(list.String(), "nope") {
		t.Fatalf("profile created after cancel: %q", list.String())
	}
}

func TestCreateGlobalInstructionsValidatesBeforeAsking(t *testing.T) {
	setupGlobalEnv(t)
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "taken"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	withTerminal(t, failAsk(t))
	for _, name := range []string{"bad name", "taken"} {
		out.Reset()
		if err := run([]string{"create", "claude", name}, &out, &out); err == nil {
			t.Fatalf("create %q succeeded", name)
		}
	}
}
