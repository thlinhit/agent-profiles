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

// maxTokenSize caps how much is read from stdin for --auth-token-stdin.
const maxTokenSize = 4096

// tokenInput is where --auth-token-stdin reads from; tests replace it.
var tokenInput = os.Stdin

func runCreate(s profile.Store, args []string, out, diagnostic io.Writer) error {
	if len(args) < 3 {
		return errors.New(commandUsage("create"))
	}
	a, err := agent.Get(args[1])
	if err != nil {
		return err
	}
	var initTemplate, tokenFromStdin bool
	var baseURL, from string
	seen := map[string]bool{}
	for i := 3; i < len(args); i++ {
		if seen[args[i]] {
			return fmt.Errorf("duplicate option %q", args[i])
		}
		seen[args[i]] = true
		switch args[i] {
		case "--init":
			initTemplate = true
		case "--auth-token-stdin":
			tokenFromStdin = true
		case "--from":
			if i+1 >= len(args) || args[i+1] == "" {
				return fmt.Errorf("--from requires a profile name")
			}
			i++
			from = args[i]
		case "--base-url":
			if i+1 >= len(args) || args[i+1] == "" {
				return fmt.Errorf("--base-url requires a value")
			}
			i++
			baseURL = args[i]
		default:
			return fmt.Errorf("unknown option %q", args[i])
		}
	}
	if initTemplate && from != "" {
		return fmt.Errorf("--init and --from cannot be combined")
	}
	if from == args[2] {
		return fmt.Errorf("--from must name a different profile")
	}
	var tokenSetter agent.AuthTokenSetter
	if tokenFromStdin {
		var ok bool
		if tokenSetter, ok = a.(agent.AuthTokenSetter); !ok {
			return fmt.Errorf("agent %q does not support --auth-token-stdin", a.Name())
		}
		// Fail before prompting rather than after the token was typed.
		if err := profile.ValidateName(args[2]); err != nil {
			return err
		}
		if _, err := s.Resolve(a.Name(), args[2]); err == nil {
			return fmt.Errorf("profile %q already exists", a.Name()+"/"+args[2])
		}
	}
	var template []byte
	switch {
	case initTemplate:
		template = a.InitTemplate()
	case from != "":
		if template, err = s.ReadSettings(a.Name(), from); err != nil {
			return err
		}
	}
	if baseURL != "" {
		b, ok := a.(agent.BaseURLSetter)
		if !ok {
			return fmt.Errorf("agent %q does not support --base-url", a.Name())
		}
		if template, err = b.WithBaseURL(template, baseURL); err != nil {
			return fmt.Errorf("--base-url: %w", err)
		}
	}
	if tokenFromStdin {
		token, err := readToken(tokenInput, diagnostic, tokenSetter.AuthTokenLabel())
		if err != nil {
			return err
		}
		if template, err = tokenSetter.WithAuthToken(template, token); err != nil {
			return err
		}
	}
	p, err := s.Create(a.Name(), args[2], template)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, p)
	return err
}

// readToken prompts without echo on a terminal, otherwise reads one line
// from piped input. The token never appears in argv or shell history.
func readToken(input *os.File, diagnostic io.Writer, label string) (string, error) {
	if term.IsTerminal(int(input.Fd())) {
		if _, err := fmt.Fprint(diagnostic, label+": "); err != nil {
			return "", err
		}
		b, err := term.ReadPassword(int(input.Fd()))
		_, _ = fmt.Fprintln(diagnostic)
		if err != nil {
			return "", fmt.Errorf("read auth token: %w", err)
		}
		if len(b) > maxTokenSize {
			return "", fmt.Errorf("auth token is longer than %d bytes", maxTokenSize)
		}
		return string(b), nil
	}
	line, err := bufio.NewReader(io.LimitReader(input, maxTokenSize+1)).ReadString('\n')
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read auth token: %w", err)
	}
	if len(line) > maxTokenSize {
		return "", fmt.Errorf("auth token is longer than %d bytes", maxTokenSize)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
