package main

import (
	"errors"
	"os"

	"github.com/thlinhit/agent-profiles/internal/dotfile"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func runLocal(s profile.Store, args []string) error {
	if len(args) != 3 {
		return errors.New(commandUsage("local"))
	}
	a, _, err := resolveProfile(s, args[1], args[2])
	if err != nil {
		return err
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	return dotfile.Write(dir, a.Name(), args[2])
}
