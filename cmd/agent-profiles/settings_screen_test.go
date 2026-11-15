package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

func press(m settingsScreen, keys ...tea.KeyMsg) settingsScreen {
	for _, k := range keys {
		next, _ := m.Update(k)
		m = next.(settingsScreen)
	}
	return m
}

var (
	keySpace = tea.KeyMsg{Type: tea.KeySpace, Runes: []rune(" ")}
	keyEnter = tea.KeyMsg{Type: tea.KeyEnter}
	keyEsc   = tea.KeyMsg{Type: tea.KeyEsc}
)

func newTestScreen(state profile.SettingState, applied *[]bool, applyErr error) settingsScreen {
	return settingsScreen{
		title: "Settings · claude/work",
		rows:  []settingsRow{{setting: profileSettings[0], initial: state, state: state}},
		apply: func(_ profileSetting, on bool) error {
			if applyErr != nil {
				return applyErr
			}
			*applied = append(*applied, on)
			return nil
		},
	}
}

func TestSettingsScreenToggleSavesEachPress(t *testing.T) {
	tests := []struct {
		name    string
		start   profile.SettingState
		keys    []tea.KeyMsg
		want    profile.SettingState
		applied []bool
	}{
		{"on to off", profile.SettingOn, []tea.KeyMsg{keySpace}, profile.SettingOff, []bool{false}},
		{"off to on with enter", profile.SettingOff, []tea.KeyMsg{keyEnter}, profile.SettingOn, []bool{true}},
		{"partial goes to on first", profile.SettingPartial, []tea.KeyMsg{keySpace}, profile.SettingOn, []bool{true}},
		{"partial never returns", profile.SettingPartial, []tea.KeyMsg{keySpace, keySpace, keySpace}, profile.SettingOn, []bool{true, false, true}},
		{"esc writes nothing", profile.SettingPartial, []tea.KeyMsg{keyEsc}, profile.SettingPartial, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var applied []bool
			m := press(newTestScreen(tt.start, &applied, nil), tt.keys...)
			if got := m.rows[0].state; got != tt.want {
				t.Fatalf("state = %s, want %s", got, tt.want)
			}
			if len(applied) != len(tt.applied) {
				t.Fatalf("applied = %v, want %v", applied, tt.applied)
			}
			for i := range applied {
				if applied[i] != tt.applied[i] {
					t.Fatalf("applied = %v, want %v", applied, tt.applied)
				}
			}
		})
	}
}

func TestSettingsScreenEscQuits(t *testing.T) {
	var applied []bool
	_, cmd := newTestScreen(profile.SettingOn, &applied, nil).Update(keyEsc)
	if cmd == nil {
		t.Fatal("esc returned no command, want tea.Quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("esc did not quit")
	}
}

func TestSettingsScreenApplyErrorKeepsState(t *testing.T) {
	var applied []bool
	m := press(newTestScreen(profile.SettingOn, &applied, errors.New("disk full")), keySpace)
	if m.rows[0].state != profile.SettingOn {
		t.Fatalf("state = %s, want on after failed save", m.rows[0].state)
	}
	if !strings.Contains(m.View(), "disk full") {
		t.Fatal("view does not show the save error")
	}
}

func TestSettingsScreenViewShowsDescription(t *testing.T) {
	var applied []bool
	view := newTestScreen(profile.SettingPartial, &applied, nil).View()
	for _, want := range []string{"Global instructions", "partial (hand-edited)", "~/.claude/CLAUDE.md", "esc close"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}
}
