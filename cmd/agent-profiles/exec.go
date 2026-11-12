package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

const execUsage = "usage: exec <agent> <name> -- <command> [args...]"

// execve replaces the current process; tests swap it out.
var execve = syscall.Exec

// runExec runs a command with one agent profile selected, without touching
// the calling shell. The command and its arguments are passed to execve
// verbatim, never through a shell.
func runExec(s profile.Store, args []string) error {
	if len(args) < 5 || args[3] != "--" {
		return fmt.Errorf(execUsage)
	}
	a, err := agent.Get(args[1])
	if err != nil {
		return err
	}
	dir, err := s.Resolve(a.Name(), args[2])
	if err != nil {
		return err
	}
	argv := args[4:]
	path, err := exec.LookPath(argv[0])
	if err != nil {
		return err
	}
	return execve(path, argv, withEnv(os.Environ(), a.EnvVar(), dir))
}

// withEnv returns env with key set to value, replacing any existing entries.
func withEnv(env []string, key, value string) []string {
	out := make([]string, 0, len(env)+1)
	for _, kv := range env {
		if !strings.HasPrefix(kv, key+"=") {
			out = append(out, kv)
		}
	}
	return append(out, key+"="+value)
}
