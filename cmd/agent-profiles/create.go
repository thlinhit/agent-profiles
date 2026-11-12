package main

import (
	"fmt"
	"io"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func runCreate(s profile.Store, args []string, out io.Writer) error {
	if len(args) < 3 || len(args) > 4 {
		return fmt.Errorf("usage: create <agent> <name> [--init|--yes]")
	}
	a, err := agent.Get(args[1])
	if err != nil {
		return err
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
