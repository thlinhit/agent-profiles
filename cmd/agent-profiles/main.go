package main

import (
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

var version = "dev"

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
	if emit && (len(args) == 0 || !emitsShell(args[0]) || commandHelpIndex(args) >= 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h") {
		out = diagnostic
	}
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		return printTopHelp(out)
	}
	if args[0] == "help" {
		if len(args) == 1 {
			return printTopHelp(out)
		}
		return printCommandHelp(out, args[1:])
	}
	if args[0] == "--version" || args[0] == "version" {
		_, err := fmt.Fprintf(out, "ap %s\n", effectiveVersion())
		return err
	}
	if helpIndex := commandHelpIndex(args); helpIndex >= 0 {
		return printCommandHelp(out, args[:helpIndex])
	}
	s := profile.Store{}
	for _, cmd := range commandRegistry {
		if cmd.name == args[0] {
			return cmd.run(s, args, out, diagnostic)
		}
	}
	return unknownCommand(args[0])
}

func effectiveVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

func commandHelpIndex(args []string) int {
	if len(args) < 2 {
		return -1
	}
	stop := len(args)
	for i, arg := range args {
		if arg == "--" {
			stop = i
			break
		}
	}
	for i := 1; i < stop; i++ {
		if args[i] == "--help" || args[i] == "-h" {
			return i
		}
	}
	return -1
}

func emitsShell(cmd string) bool { return strings.Contains(" init use off auto ", " "+cmd+" ") }
