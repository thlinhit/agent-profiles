package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// profileSetting is a Profile setting ap defines. To add one, append an entry
// to profileSettings and give the Agents that support it the matching
// capability. Values are on/off; widen the shape here if a setting needs more.
type profileSetting struct {
	key, label, summary string
	// description is the longer help shown under the list in the settings screen.
	description string
	supports    func(agent.Agent) bool
	read        func(a agent.Agent, settings []byte, home string) (profile.SettingState, error)
	write       func(a agent.Agent, settings []byte, home string, on bool) ([]byte, error)
}

var profileSettings = []profileSetting{
	{
		key:     "global-instructions",
		label:   "Global instructions",
		summary: "Load instruction files from your home directory (CLAUDE.md, rules, AGENTS.md)",
		description: "Load your home instruction files in this Profile: ~/.claude/CLAUDE.md, ~/.claude/rules/, ~/AGENTS.md and 3 more.\n" +
			"Off: only this Profile's CLAUDE.md and project files load.",
		supports: func(a agent.Agent) bool {
			_, ok := a.(agent.GlobalInstructionsSetter)
			return ok
		},
		read: func(a agent.Agent, settings []byte, home string) (profile.SettingState, error) {
			return a.(agent.GlobalInstructionsSetter).GlobalInstructionsState(settings, home)
		},
		write: func(a agent.Agent, settings []byte, home string, on bool) ([]byte, error) {
			return a.(agent.GlobalInstructionsSetter).WithGlobalInstructions(settings, home, on)
		},
	},
}

func settingKeys() []string {
	keys := make([]string, 0, len(profileSettings))
	for _, s := range profileSettings {
		keys = append(keys, s.key)
	}
	return keys
}

func findProfileSetting(key string) (profileSetting, error) {
	for _, s := range profileSettings {
		if s.key == key {
			return s, nil
		}
	}
	return profileSetting{}, fmt.Errorf("unknown setting %q. Known settings: %s", key, strings.Join(settingKeys(), ", "))
}

func unsupportedSettingErr(a agent.Agent, s profileSetting) error {
	return fmt.Errorf("%s does not support %s yet", a.Name(), s.label)
}

// supportedSettings returns the Profile settings a has, in registry order.
func supportedSettings(a agent.Agent) []profileSetting {
	var out []profileSetting
	for _, s := range profileSettings {
		if s.supports(a) {
			out = append(out, s)
		}
	}
	return out
}

// applyProfileSetting sets s to on in the Profile directory dir. It writes
// only when the state differs and reports whether it wrote.
func applyProfileSetting(a agent.Agent, dir, label string, s profileSetting, on bool) (bool, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	current, err := profile.ReadSettings(dir, label)
	if err != nil {
		return false, err
	}
	state, err := s.read(a, current, home)
	if err != nil {
		return false, fmt.Errorf("%s: %w", label, err)
	}
	want := profile.SettingOff
	if on {
		want = profile.SettingOn
	}
	if state == want {
		return false, nil
	}
	next, err := s.write(a, current, home, on)
	if err != nil {
		return false, fmt.Errorf("%s: %w", label, err)
	}
	if err := profile.WriteSettings(dir, label, next); err != nil {
		return false, err
	}
	return true, nil
}

// reportSetting prints the outcome of applyProfileSetting.
func reportSetting(out io.Writer, label string, s profileSetting, on, changed bool) error {
	if !changed {
		_, err := fmt.Fprintf(out, "%s: %s unchanged\n", label, s.key)
		return err
	}
	value := "off"
	if on {
		value = "on"
	}
	_, err := fmt.Fprintf(out, "%s: %s %s (restart running sessions to apply)\n", label, s.key, value)
	return err
}

func runSettings(s profile.Store, args []string, out io.Writer) error {
	if len(args) == 1 {
		return runSettingsForm(s, nil, out)
	}
	jsonOutput := false
	rest := make([]string, 0, len(args))
	rest = append(rest, args[1])
	for _, arg := range args[2:] {
		if arg == "--json" {
			jsonOutput = true
		} else {
			rest = append(rest, arg)
		}
	}
	const jsonOnly = "--json works with settings ls and settings get only"
	switch rest[0] {
	case "ls":
		if len(rest) != 3 {
			return errors.New(commandUsage("settings"))
		}
		return runSettingsLs(s, rest[1:], jsonOutput, out)
	case "get":
		if len(rest) != 4 {
			return errors.New(commandUsage("settings"))
		}
		return runSettingsGet(s, rest[1:], jsonOutput, out)
	case "set":
		if jsonOutput {
			return errors.New(jsonOnly)
		}
		if len(rest) != 5 {
			return errors.New(commandUsage("settings"))
		}
		return runSettingsSet(s, rest[1:], out)
	}
	if jsonOutput {
		return errors.New(jsonOnly)
	}
	for _, arg := range args[1:] {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown option %q", arg)
		}
	}
	if len(args) > 3 {
		return errors.New(commandUsage("settings"))
	}
	return runSettingsForm(s, args[1:], out)
}

// settingsProfile resolves the Agent and Profile and reads the Profile's settings.
func settingsProfile(s profile.Store, agentName, profileName string) (agent.Agent, string, []byte, error) {
	a, err := agent.Get(agentName)
	if err != nil {
		return nil, "", nil, err
	}
	dir, err := s.Resolve(a.Name(), profileName)
	if err != nil {
		return nil, "", nil, err
	}
	label := a.Name() + "/" + profileName
	current, err := profile.ReadSettings(dir, label)
	if err != nil {
		return nil, "", nil, err
	}
	return a, label, current, nil
}

type settingRow struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

func writeJSON(out io.Writer, v any) error {
	enc := json.NewEncoder(out)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// runSettingsLs args: <agent> <profile>.
func runSettingsLs(s profile.Store, args []string, jsonOutput bool, out io.Writer) error {
	a, label, current, err := settingsProfile(s, args[0], args[1])
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	rows := []settingRow{}
	for _, set := range supportedSettings(a) {
		state, err := set.read(a, current, home)
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		rows = append(rows, settingRow{set.key, string(state)})
	}
	if jsonOutput {
		return writeJSON(out, rows)
	}
	if len(rows) == 0 {
		_, err := fmt.Fprintf(out, "no Profile settings for %s\n", a.Name())
		return err
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%s\t%s\n", r.Key, r.Value); err != nil {
			return err
		}
	}
	return w.Flush()
}

// runSettingsGet args: <agent> <profile> <key>.
func runSettingsGet(s profile.Store, args []string, jsonOutput bool, out io.Writer) error {
	set, err := findProfileSetting(args[2])
	if err != nil {
		return err
	}
	a, label, current, err := settingsProfile(s, args[0], args[1])
	if err != nil {
		return err
	}
	if !set.supports(a) {
		return unsupportedSettingErr(a, set)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	state, err := set.read(a, current, home)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	if jsonOutput {
		return writeJSON(out, settingRow{set.key, string(state)})
	}
	_, err = fmt.Fprintln(out, state)
	return err
}

// runSettingsSet args: <agent> <profile> <key> <on|off>.
func runSettingsSet(s profile.Store, args []string, out io.Writer) error {
	set, err := findProfileSetting(args[2])
	if err != nil {
		return err
	}
	if args[3] != "on" && args[3] != "off" {
		return fmt.Errorf("invalid value %q for %s: want on or off", args[3], set.key)
	}
	a, err := agent.Get(args[0])
	if err != nil {
		return err
	}
	if !set.supports(a) {
		return unsupportedSettingErr(a, set)
	}
	dir, err := s.Resolve(a.Name(), args[1])
	if err != nil {
		return err
	}
	label := a.Name() + "/" + args[1]
	on := args[3] == "on"
	changed, err := applyProfileSetting(a, dir, label, set, on)
	if err != nil {
		return err
	}
	return reportSetting(out, label, set, on, changed)
}
