package main

import (
	"fmt"
	"github.com/linhtruong/agent-profiles/internal/agent"
	"github.com/linhtruong/agent-profiles/internal/profile"
	"io"
	"os"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string, out, diagnostic io.Writer) error {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" {
		_, err := fmt.Fprintln(out, "agent-profiles: init use off current list create delete local trust untrust")
		return err
	}
	s := profile.Store{}
	switch args[0] {
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
				fmt.Fprintf(out, "%s %s\n", a.Name(), n)
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
			if len(args) != 4 || args[3] != "--yes" {
				return fmt.Errorf("delete requires --yes confirmation")
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
