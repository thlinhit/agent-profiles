package main

import (
	"fmt"
	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/dotfile"
	"github.com/thlinhit/agent-profiles/internal/profile"
	"github.com/thlinhit/agent-profiles/internal/shell"
	"github.com/thlinhit/agent-profiles/internal/trust"
	"io"
	"os"
	"strings"
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
	for _, arg := range args {
		if arg == "--emit-shell" {
			emit = true
		} else {
			filtered = append(filtered, arg)
		}
	}
	args = filtered
	if emit && (len(args) == 0 || (args[0] != "init" && args[0] != "use" && args[0] != "off" && args[0] != "auto")) {
		out = diagnostic
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := fmt.Fprintln(out, "agent-profiles: init use off current list create delete local trust untrust")
		return err
	}
	s := profile.Store{}
	switch args[0] {
	case "trust", "untrust":
		if len(args) != 1 {
			return fmt.Errorf("usage: %s", args[0])
		}
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		dir, err = dotfile.Canonical(dir)
		if err != nil {
			return err
		}
		if args[0] == "untrust" {
			return (trust.Store{}).Revoke(dir)
		}
		doc, err := dotfile.Find(dir)
		if err != nil {
			return err
		}
		if doc == nil || doc.Dir != dir {
			return fmt.Errorf("trust requires a dotfile in the current directory")
		}
		// Approval binds raw bytes only; resolution validates TOML after trust.
		return (trust.Store{}).Grant(doc)
	case "auto":
		if len(args) != 1 {
			return fmt.Errorf("usage: auto")
		}
		paths, err := selections(diagnostic)
		if err != nil {
			return err
		}
		var code strings.Builder
		for _, a := range agent.All() {
			if p, ok := paths[a.Name()]; ok {
				line, err := shell.Export(a.EnvVar(), p)
				if err != nil {
					return err
				}
				code.WriteString(line)
			}
		}
		_, err = io.WriteString(out, code.String())
		return err
	case "local":
		if len(args) != 3 {
			return fmt.Errorf("usage: local <agent> <name>")
		}
		a, err := agent.Get(args[1])
		if err != nil {
			return err
		}
		if _, err = s.Resolve(a.Name(), args[2]); err != nil {
			return err
		}
		dir, err := os.Getwd()
		if err != nil {
			return err
		}
		return dotfile.Write(dir, a.Name(), args[2])
	case "init":
		if len(args) != 2 {
			return fmt.Errorf("usage: init zsh|bash")
		}
		code, err := shell.Init(args[1])
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, code)
		return err
	case "use", "off":
		want := 3
		if args[0] == "off" {
			want = 2
		}
		if len(args) != want {
			return fmt.Errorf("usage: %s <agent> [name]", args[0])
		}
		a, err := agent.Get(args[1])
		if err != nil {
			return err
		}
		var code string
		if args[0] == "off" {
			code, err = shell.Unset(a.EnvVar())
		} else {
			var p string
			p, err = s.Resolve(a.Name(), args[2])
			if err == nil {
				code, err = shell.Export(a.EnvVar(), p)
			}
		}
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, code)
		return err
	case "current":
		if len(args) != 1 {
			return fmt.Errorf("usage: current")
		}
		paths, err := selections(diagnostic)
		if err != nil {
			return err
		}
		for _, a := range agent.All() {
			value, source := os.Getenv(a.EnvVar()), "env"
			if value == "" {
				if p, ok := paths[a.Name()]; ok {
					value = p
					source = "dotfile"
				}
			}
			if value == "" {
				source = "default"
				var err error
				value, err = a.DefaultConfigDir()
				if err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(out, "%s %s (%s)\n", a.Name(), value, source); err != nil {
				return err
			}
		}
		return nil
	case "list":
		if len(args) > 2 {
			return fmt.Errorf("usage: list [agent]")
		}
		agents := agent.All()
		if len(args) == 2 {
			a, err := agent.Get(args[1])
			if err != nil {
				return err
			}
			agents = []agent.Agent{a}
		}
		for _, a := range agents {
			names, err := s.List(a.Name())
			if err != nil {
				return err
			}
			for _, n := range names {
				if _, err := fmt.Fprintf(out, "%s %s\n", a.Name(), n); err != nil {
					return err
				}
			}
		}
		return nil
	case "create", "delete":
		if len(args) < 3 || len(args) > 4 {
			return fmt.Errorf("usage: %s <agent> <name> [--init|--yes]", args[0])
		}
		a, err := agent.Get(args[1])
		if err != nil {
			return err
		}
		if args[0] == "delete" {
			if len(args) == 4 && args[3] != "--yes" {
				return fmt.Errorf("unknown option %q", args[3])
			}
			if _, err := s.Resolve(a.Name(), args[2]); err != nil {
				return err
			}
			if len(args) == 3 {
				if err := confirmDelete(os.Stdin, diagnostic, a.Name(), args[2]); err != nil {
					return err
				}
			}
			return s.Delete(a.Name(), args[2])
		}
		var template []byte
		if len(args) == 4 {
			if args[3] != "--init" {
				return fmt.Errorf("unknown option %q", args[3])
			}
			template = a.InitTemplate()
		}
		p, err := s.Create(a.Name(), args[2], template)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(out, p)
		return err
	}
	return fmt.Errorf("unknown command %q", args[0])
}
