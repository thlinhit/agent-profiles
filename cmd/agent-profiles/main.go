package main

import (
	"fmt"
	"io"
	"os"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, out, diagnostic io.Writer) error {
	emit := false
	filtered := make([]string, 0, len(args))
	for i, arg := range args {
		if arg == "--" {
			// Everything after "--" belongs to another program (ap exec).
			filtered = append(filtered, args[i:]...)
			break
		}
		if arg == "--emit-shell" {
			emit = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	args = filtered
	if emit && (len(args) == 0 || !emitsShell(args[0])) {
		out = diagnostic
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := fmt.Fprintln(out, "agent-profiles: init use off current list create update delete exec local trust untrust")
		return err
	}
	s := profile.Store{}
	switch args[0] {
	case "trust", "untrust":
		return runTrust(args)
	case "auto":
		return runAuto(args, out, diagnostic)
	case "local":
		return runLocal(s, args)
	case "init":
		return runInit(args, out)
	case "use", "off":
		return runUse(s, args, out)
	case "current":
		return runCurrent(args, out, diagnostic)
	case "list":
		return runList(s, args, out)
	case "create":
		return runCreate(s, args, out, diagnostic)
	case "delete":
		return runDelete(s, args, diagnostic)
	case "update":
		return runUpdate(s, args, out)
	case "exec":
		return runExec(s, args)
	}
	return fmt.Errorf("unknown command %q", args[0])
}

// emitsShell reports whether a command writes shell code to stdout for eval.
func emitsShell(cmd string) bool {
	switch cmd {
	case "init", "use", "off", "auto":
		return true
	}
	return false
}
