package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/agent"
)

func TestTopHelp(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"--help"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	for _, c := range commandRegistry {
		if !c.hidden && !strings.Contains(out.String(), c.name) {
			t.Errorf("help missing %q", c.name)
		}
	}
	if strings.Contains(out.String(), "auto") {
		t.Errorf("hidden command shown: %s", out.String())
	}
	for _, a := range agent.All() {
		if !strings.Contains(out.String(), a.Name()) {
			t.Errorf("help missing agent %q", a.Name())
		}
	}
}

func TestEveryCommandHelp(t *testing.T) {
	for _, c := range commandRegistry {
		for _, args := range [][]string{{c.name, "--help"}, {"help", c.name}} {
			var out bytes.Buffer
			if err := run(args, &out, &out); err != nil {
				t.Errorf("%v: %v", args, err)
			}
			if !strings.Contains(out.String(), "Usage: ap ") {
				t.Errorf("%v: missing usage: %q", args, out.String())
			}
		}
	}
}

func TestEmitShellHelpUsesDiagnostic(t *testing.T) {
	var out, diag bytes.Buffer
	if err := run([]string{"use", "--help", "--emit-shell"}, &out, &diag); err != nil {
		t.Fatal(err)
	}
	if out.Len() != 0 || diag.Len() == 0 {
		t.Fatalf("out=%q diag=%q", out.String(), diag.String())
	}
}

func TestExecHelpPassthrough(t *testing.T) {
	called := false
	old := execve
	execve = func(path string, argv, env []string) error {
		called = true
		if len(argv) < 2 || argv[len(argv)-1] != "--help" {
			t.Errorf("argv %v", argv)
		}
		return errors.New("stop")
	}
	defer func() { execve = old }()
	// Invalid profile resolution is avoided by checking that help parsing respects --;
	// execution itself is covered by TestExecPassthroughWithArgs.
	if commandHelpIndex([]string{"exec", "claude", "work", "--", "foo", "--help"}) >= 0 {
		t.Fatal("treated passthrough --help as CLI help")
	}
	_ = called
}

func TestHelpExamplesAndWidth(t *testing.T) {
	var top bytes.Buffer
	if err := run([]string{"--help"}, &top, &top); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(top.String(), "\n") {
		if len(line) > 80 {
			t.Errorf("help line is %d columns: %q", len(line), line)
		}
	}
	for _, c := range commandRegistry {
		if c.hidden {
			continue
		}
		var out bytes.Buffer
		if err := run([]string{"help", c.name}, &out, &out); err != nil {
			t.Errorf("%s: %v", c.name, err)
		}
		if !strings.Contains(out.String(), "Examples:") {
			t.Errorf("%s lacks examples: %s", c.name, out.String())
		}
	}
}

func TestAgentArgumentListsRegisteredAgents(t *testing.T) {
	for _, name := range []string{"use", "off", "delete", "update"} {
		var out bytes.Buffer
		if err := run([]string{"help", name}, &out, &out); err != nil {
			t.Fatal(err)
		}
		line := ""
		for _, l := range strings.Split(out.String(), "\n") {
			if strings.HasPrefix(strings.TrimSpace(l), "<agent>") {
				line = l
			}
		}
		if want := "One of: " + strings.Join(agentNames(), ", "); !strings.Contains(line, want) {
			t.Errorf("%s: agent line %q lacks %q", name, line, want)
		}
		for _, a := range agent.All() {
			if !strings.Contains(line, a.Name()) {
				t.Errorf("%s: agent line %q lacks agent %q", name, line, a.Name())
			}
		}
	}
}

func TestAutoHelpHasSummaryAndNoExamples(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"help", "auto"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Print exports for trusted Dotfile selections in this directory") {
		t.Errorf("auto help lacks summary: %s", out.String())
	}
	if strings.Contains(out.String(), "Examples:") {
		t.Errorf("auto help has examples: %s", out.String())
	}
}

func TestSkillsPoolUsageError(t *testing.T) {
	err := run([]string{"skills", "pool"}, &bytes.Buffer{}, &bytes.Buffer{})
	want := "usage: ap skills pool <add|rm|ls>\nRun 'ap help skills pool' for details."
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestSkillsHelpFlagsAndUsageErrors(t *testing.T) {
	for _, sub := range []string{"add", "rm", "select", "pool add", "pool rm"} {
		var out bytes.Buffer
		args := append([]string{"help", "skills"}, strings.Fields(sub)...)
		if err := run(args, &out, &out); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "--json") {
			t.Errorf("%s help has unrelated --json", sub)
		}
	}
	if usage := skillUsage("add"); !strings.HasPrefix(usage, "usage: ap skills add ") || !strings.Contains(usage, "Run 'ap help skills add' for details.") {
		t.Errorf("skills add usage: %q", usage)
	}
	for _, args := range [][]string{{"list", "a", "b"}, {"create"}} {
		err := run(args, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || !strings.HasPrefix(err.Error(), "usage: ap ") || !strings.Contains(err.Error(), "Run 'ap help ") {
			t.Errorf("%v: %v", args, err)
		}
	}
	for _, args := range [][]string{{"skills", "lss"}, {"skills", "pool", "lss"}} {
		err := run(args, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), `Did you mean "ls"?`) {
			t.Errorf("%v: %v", args, err)
		}
	}
}

func TestUnknownSuggestionAndVersion(t *testing.T) {
	if err := run([]string{"lsit"}, &bytes.Buffer{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), `Did you mean "list"?`) {
		t.Fatalf("suggestion: %v", err)
	}
	old := version
	version = "1.2.3"
	defer func() { version = old }()
	var out bytes.Buffer
	if err := run([]string{"--version"}, &out, &bytes.Buffer{}); err != nil || out.String() != "ap 1.2.3\n" {
		t.Fatalf("version %q: %v", out.String(), err)
	}
}
