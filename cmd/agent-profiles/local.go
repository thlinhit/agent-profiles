package main

import (
	"fmt"
	"os"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/dotfile"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func runLocal(s profile.Store, args []string) error {
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
}
