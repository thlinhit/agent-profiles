package main

import (
	"fmt"
	"io"
	"os"

	"github.com/thlinhit/agent-profiles/internal/agent"
)

func runCurrent(args []string, out, diagnostic io.Writer) error {
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
}
