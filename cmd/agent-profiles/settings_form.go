package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// runSettingsForm is the interactive `ap settings [<agent> [<profile>]]`.
// The form is only UI: it collects one choice per Profile setting and then
// writes the changed ones through the same path as `ap settings set`.
func runSettingsForm(s profile.Store, args []string, out io.Writer) error {
	// Like skills select: prompts render on stderr, so stdin and stderr must be terminals.
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return errors.New("ap settings needs a terminal; use ap settings get or ap settings set")
	}
	lipgloss.SetDefaultRenderer(lipgloss.NewRenderer(os.Stderr))
	var agentName, profileName string
	if len(args) > 0 {
		agentName = args[0]
	}
	if len(args) > 1 {
		profileName = args[1]
	}
	if agentName == "" {
		var opts []huh.Option[string]
		for _, a := range agent.All() {
			if len(supportedSettings(a)) > 0 {
				opts = append(opts, huh.NewOption(a.Name(), a.Name()))
			}
		}
		if err := runPrompt(huh.NewSelect[string]().Title("Select agent").Options(opts...).Value(&agentName)); err != nil {
			return abortErr(err, out)
		}
	}
	a, err := agent.Get(agentName)
	if err != nil {
		return err
	}
	if len(supportedSettings(a)) == 0 {
		return unsupportedSettingErr(a, profileSettings[0])
	}
	if profileName == "" {
		names, err := s.List(a.Name())
		if err != nil {
			return err
		}
		if len(names) == 0 {
			return fmt.Errorf("no %s profiles; run ap create %s <name> first", a.Name(), a.Name())
		}
		if err := runPrompt(huh.NewSelect[string]().Title("Select profile (" + a.Name() + ")").Options(huh.NewOptions(names...)...).Value(&profileName)); err != nil {
			return abortErr(err, out)
		}
	}
	dir, err := s.Resolve(a.Name(), profileName)
	if err != nil {
		return err
	}
	label := a.Name() + "/" + profileName
	current, err := profile.ReadSettings(dir, label)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}

	screen := settingsScreen{
		title: "Settings · " + label,
		apply: func(set profileSetting, on bool) error {
			_, err := applyProfileSetting(a, dir, label, set, on)
			return err
		},
	}
	for _, set := range supportedSettings(a) {
		state, err := set.read(a, current, home)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		screen.rows = append(screen.rows, settingsRow{setting: set, initial: state, state: state})
	}
	final, err := tea.NewProgram(screen, tea.WithOutput(os.Stderr)).Run()
	if err != nil {
		return err
	}
	// Each toggle was saved when pressed; report only settings that ended up changed.
	for _, r := range final.(settingsScreen).rows {
		if r.state == r.initial {
			continue
		}
		if err := reportSetting(out, label, r.setting, r.state == profile.SettingOn, true); err != nil {
			return err
		}
	}
	return nil
}
