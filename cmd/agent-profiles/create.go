package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
	"golang.org/x/term"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// maxTokenSize caps how much is read from stdin for --auth-token-stdin.
const maxTokenSize = 4096

// tokenInput is where --auth-token-stdin reads from; tests replace it.
var tokenInput = os.Stdin

// stdinIsTerminal reports whether create may ask a question; tests replace it.
var stdinIsTerminal = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// stderrIsTerminal reports whether the question would be visible; prompts render on stderr. Tests replace it.
var stderrIsTerminal = func() bool { return term.IsTerminal(int(os.Stderr.Fd())) }

// askGlobalInstructions asks whether to load Global instructions; tests replace it.
var askGlobalInstructions = askGlobalInstructionsOn

var errCancelled = errors.New("cancelled, no profile created")

func runCreate(s profile.Store, args []string, out, diagnostic io.Writer) error {
	if len(args) < 3 {
		return errors.New(commandUsage("create"))
	}
	a, err := agent.Get(args[1])
	if err != nil {
		return err
	}
	var initTemplate, tokenFromStdin bool
	var baseURL, from, globalFlag string
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
		case "--global-instructions":
			if i+1 >= len(args) || args[i+1] == "" {
				return fmt.Errorf("--global-instructions requires on or off")
			}
			i++
			globalFlag = args[i]
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
	if globalFlag != "" && globalFlag != "on" && globalFlag != "off" {
		return fmt.Errorf("--global-instructions must be on or off, not %q", globalFlag)
	}
	globalSetter, hasGlobal := a.(agent.GlobalInstructionsSetter)
	if globalFlag != "" && !hasGlobal {
		return fmt.Errorf("%s does not support Global instructions yet", a.Name())
	}
	// Ask only when nothing else decides the setting: --from copies it and
	// the flag overrides it. Both streams must be terminals, as in the settings
	// screen: an unseen question would block.
	askGlobal := globalFlag == "" && from == "" && hasGlobal && stdinIsTerminal() && stderrIsTerminal()
	var tokenSetter agent.AuthTokenSetter
	if tokenFromStdin {
		var ok bool
		if tokenSetter, ok = a.(agent.AuthTokenSetter); !ok {
			return fmt.Errorf("agent %q does not support --auth-token-stdin", a.Name())
		}
	}
	if tokenFromStdin || askGlobal {
		// Fail before prompting rather than after the answer was typed.
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
	var globalOn, setGlobal bool
	switch {
	case globalFlag != "":
		globalOn, setGlobal = globalFlag == "on", true
	case askGlobal:
		if globalOn, err = askGlobalInstructions(); err != nil {
			return err
		}
		setGlobal = true
	}
	if setGlobal {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		if template, err = globalSetter.WithGlobalInstructions(template, home, globalOn); err != nil {
			return fmt.Errorf("global instructions: %w", err)
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

// askGlobalInstructionsOn asks on stderr and defaults to Yes.
func askGlobalInstructionsOn() (bool, error) {
	on := true
	err := runPrompt(huh.NewConfirm().
		Title("Load Global instructions?").
		Description("Instruction files from your home directory (CLAUDE.md, rules, AGENTS.md).").
		Affirmative("Yes").
		Negative("No").
		Value(&on))
	if errors.Is(err, huh.ErrUserAborted) {
		return false, errCancelled
	}
	return on, err
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
