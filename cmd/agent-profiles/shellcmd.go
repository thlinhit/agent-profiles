package main

import (
	"errors"
	"io"
	"strings"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
	"github.com/thlinhit/agent-profiles/internal/shell"
)

// Commands in this file print shell code meant to be eval'd by the shell hook.

func runInit(args []string, out io.Writer) error {
	if len(args) != 2 {
		return errors.New(commandUsage("init"))
	}
	code, err := shell.Init(args[1])
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, code)
	return err
}

func runAuto(args []string, out, diagnostic io.Writer) error {
	if len(args) != 1 {
		return errors.New(commandUsage("auto"))
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
}

func runUse(s profile.Store, args []string, out io.Writer) error {
	want := 3
	if args[0] == "off" {
		want = 2
	}
	if len(args) != want {
		return errors.New(commandUsage(args[0]))
	}
	var code string
	var err error
	if args[0] == "off" {
		a, getErr := agent.Get(args[1])
		if getErr != nil {
			return getErr
		}
		code, err = shell.Unset(a.EnvVar())
	} else {
		a, p, resolveErr := resolveProfile(s, args[1], args[2])
		if resolveErr != nil {
			return resolveErr
		}
		code, err = shell.Export(a.EnvVar(), p)
	}
	if err != nil {
		return err
	}
	_, err = io.WriteString(out, code)
	return err
}
