package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
	"github.com/thlinhit/agent-profiles/internal/shell"
)

// Commands in this file print shell code meant to be eval'd by the shell hook.

func runInit(args []string, out io.Writer) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: init zsh|bash")
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
}

func runUse(s profile.Store, args []string, out io.Writer) error {
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
}
