package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func runDelete(s profile.Store, args []string, diagnostic io.Writer) error {
	if len(args) < 3 || len(args) > 4 {
		return errors.New(commandUsage("delete"))
	}
	if len(args) == 4 && args[3] != "--yes" {
		// Unknown Agents take precedence over invalid options.
		if _, err := agent.Get(args[1]); err != nil {
			return err
		}
		return fmt.Errorf("unknown option %q", args[3])
	}
	a, _, err := resolveProfile(s, args[1], args[2])
	if err != nil {
		return err
	}
	if len(args) == 3 {
		if err := confirmDelete(os.Stdin, diagnostic, a.Name(), args[2]); err != nil {
			return err
		}
	}
	return s.Delete(a.Name(), args[2])
}

func confirmDelete(input *os.File, diagnostic io.Writer, agent, name string) error {
	if !term.IsTerminal(int(input.Fd())) {
		return fmt.Errorf("delete requires --yes when stdin is not a terminal; use --yes to delete non-interactively")
	}
	if _, err := fmt.Fprintf(diagnostic, "Delete profile %q for %q? [y/N] ", name, agent); err != nil {
		return err
	}
	answer, err := bufio.NewReader(io.LimitReader(input, 1024)).ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("deletion cancelled")
		}
		return fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	default:
		return fmt.Errorf("deletion cancelled")
	}
}
