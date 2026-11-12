package main

import (
	"fmt"
	"io"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func runList(s profile.Store, args []string, out io.Writer) error {
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
}
