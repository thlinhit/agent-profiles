package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func stubExecve(t *testing.T) *struct {
	path string
	argv []string
	env  []string
} {
	t.Helper()
	got := &struct {
		path string
		argv []string
		env  []string
	}{}
	old := execve
	execve = func(path string, argv, env []string) error {
		got.path, got.argv, got.env = path, argv, env
		return nil
	}
	t.Cleanup(func() { execve = old })
	return got
}

func TestExec(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("CLAUDE_CONFIG_DIR", "/previous")
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	dir := strings.TrimSpace(out.String())
	got := stubExecve(t)
	// Arguments after "--" are passed verbatim, including ones the tool
	// itself would otherwise interpret.
	args := []string{"exec", "claude", "work", "--", "sh", "-c", "echo $0", "--emit-shell", "; rm -rf /"}
	if err := run(args, &out, &out); err != nil {
		t.Fatal(err)
	}
	if want, _ := exec.LookPath("sh"); got.path != want {
		t.Fatalf("path = %q; want %q", got.path, want)
	}
	if !reflect.DeepEqual(got.argv, args[4:]) {
		t.Fatalf("argv = %q; want %q", got.argv, args[4:])
	}
	var vals []string
	for _, kv := range got.env {
		if strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") {
			vals = append(vals, kv)
		}
	}
	if len(vals) != 1 || vals[0] != "CLAUDE_CONFIG_DIR="+dir {
		t.Fatalf("CLAUDE_CONFIG_DIR entries = %q; want one for %s", vals, dir)
	}
}

func TestExecRejected(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	got := stubExecve(t)
	for name, args := range map[string][]string{
		"no separator":    {"exec", "claude", "work", "sh"},
		"no command":      {"exec", "claude", "work", "--"},
		"unknown agent":   {"exec", "nope", "work", "--", "sh"},
		"missing profile": {"exec", "claude", "nope", "--", "sh"},
		"bad name":        {"exec", "claude", "../work", "--", "sh"},
		"not found":       {"exec", "claude", "work", "--", "no-such-command-xyz"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := run(args, &out, &out); err == nil {
				t.Fatalf("run(%q) succeeded; want error", args)
			}
			if got.path != "" {
				t.Fatalf("execve called for %q", args)
			}
		})
	}
}

// TestExecThroughShellFunction runs the real binary through the ap function
// to prove exec is not captured or evaluated and leaves the shell unchanged.
func TestExecThroughShellFunction(t *testing.T) {
	tmp := t.TempDir()
	bin := filepath.Join(tmp, "bin")
	build := exec.Command("go", "build", "-o", filepath.Join(bin, "agent-profiles"), ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	data := filepath.Join(tmp, "data")
	for _, sh := range []string{"zsh", "bash"} {
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		t.Run(sh, func(t *testing.T) {
			script := `eval "$(agent-profiles init ` + sh + `)"
agent-profiles create claude work >/dev/null 2>&1
ap exec claude work -- sh -c 'printf "%s|%s\n" "$CLAUDE_CONFIG_DIR" "$1"' x '--emit-shell'
echo "status=$? after=${CLAUDE_CONFIG_DIR-unset}"`
			cmd := exec.Command(sh, "-c", script)
			cmd.Dir = tmp
			for _, kv := range os.Environ() {
				if !strings.HasPrefix(kv, "CLAUDE_CONFIG_DIR=") {
					cmd.Env = append(cmd.Env, kv)
				}
			}
			cmd.Env = append(cmd.Env, "PATH="+bin+":"+os.Getenv("PATH"), "XDG_DATA_HOME="+data, "HOME="+tmp)
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%v\n%s", err, out)
			}
			want := filepath.Join(data, "agent-profiles", "claude", "work") + "|--emit-shell\nstatus=0 after=unset\n"
			if string(out) != want {
				t.Fatalf("output = %q; want %q", out, want)
			}
		})
	}
}
