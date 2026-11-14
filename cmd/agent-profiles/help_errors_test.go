package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestSkillsBareCommandIsUsageError(t *testing.T) {
	err := run([]string{"skills"}, &bytes.Buffer{}, &bytes.Buffer{})
	want := "usage: ap skills <ls|add|rm|select|pool>\nRun 'ap help skills' for details."
	if err == nil || err.Error() != want {
		t.Fatalf("got %v, want %q", err, want)
	}
}

func TestSkillsValidSubcommandWrongArgsIsUsageError(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"skills", "ls"}, "usage: ap skills ls <agent> <profile> [--json]\nRun 'ap help skills ls' for details."},
		{[]string{"skills", "ls", "claude"}, "usage: ap skills ls <agent> <profile> [--json]\nRun 'ap help skills ls' for details."},
		{[]string{"skills", "ls", "--pool", "extra"}, "usage: ap skills ls --pool [--json]\nRun 'ap help skills ls --pool' for details."},
		{[]string{"skills", "add"}, "usage: ap skills add <agent> <profile> <path-or-name> [--name N]\nRun 'ap help skills add' for details."},
		{[]string{"skills", "rm", "x"}, "usage: ap skills rm <agent> <profile> <skill>\nRun 'ap help skills rm' for details."},
	}
	for _, tc := range tests {
		err := run(tc.args, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || err.Error() != tc.want {
			t.Errorf("%v: got %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestSkillsUnknownSubcommandErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"skills", "lss"}, "unknown skills subcommand \"lss\". Did you mean \"ls\"?\nRun 'ap help skills' for a list of subcommands."},
		{[]string{"skills", "zzzzzzzz"}, "unknown skills subcommand \"zzzzzzzz\".\nRun 'ap help skills' for a list of subcommands."},
		{[]string{"skills", "pool", "lss"}, "unknown skills subcommand \"lss\". Did you mean \"ls\"?\nRun 'ap help skills pool' for a list of subcommands."},
		{[]string{"skills", "pool", "zzzzzzzz"}, "unknown skills subcommand \"zzzzzzzz\".\nRun 'ap help skills pool' for a list of subcommands."},
		{[]string{"help", "skills", "lss"}, "unknown skills subcommand \"lss\". Did you mean \"ls\"?\nRun 'ap help skills' for a list of subcommands."},
		{[]string{"help", "skills", "pool", "lss"}, "unknown skills subcommand \"lss\". Did you mean \"ls\"?\nRun 'ap help skills pool' for a list of subcommands."},
	}
	for _, tc := range tests {
		err := run(tc.args, &bytes.Buffer{}, &bytes.Buffer{})
		if err == nil || err.Error() != tc.want {
			t.Errorf("%v: got %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestHelpSynopsesAndDescriptions(t *testing.T) {
	var top bytes.Buffer
	if err := run([]string{"--help"}, &top, &top); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(top.String(), "exec <agent> <name> [-- <cmd>]") {
		t.Errorf("top help exec synopsis: %s", top.String())
	}

	var add bytes.Buffer
	if err := run([]string{"help", "skills", "add"}, &add, &add); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(add.String(), "Skill source path, or Skill name found in configured pools") {
		t.Errorf("skills add help: %s", add.String())
	}
}

func TestHelpNotes(t *testing.T) {
	notes := map[string]string{
		"local":   "Writes .agent-profile here. Run 'ap trust' next; it has no effect until trusted.",
		"trust":   "Trust is bound to the file's exact bytes; re-run after any edit. The shell hook applies it on the next prompt in bash or the next directory change in zsh; cd . applies it now in either shell.",
		"untrust": "Revokes Trust; it does not deactivate profiles already exported in this shell. Use 'ap off <agent>'.",
	}
	for cmd, note := range notes {
		var out bytes.Buffer
		if err := run([]string{"help", cmd}, &out, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out.String(), "Notes: "+note+"\n") {
			t.Errorf("help %s: missing note %q in %q", cmd, note, out.String())
		}
	}
}
