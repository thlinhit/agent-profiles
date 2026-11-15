package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

// settingsRow is one Profile setting in the settings screen.
type settingsRow struct {
	setting profileSetting
	initial profile.SettingState
	state   profile.SettingState
}

// settingsScreen is the interactive list behind `ap settings`, modelled on
// Claude Code's /config: one row per Profile setting, Space/Enter toggles and
// saves at once, Esc closes. It holds no I/O; apply does the writing.
type settingsScreen struct {
	title  string
	rows   []settingsRow
	cursor int
	err    error
	apply  func(s profileSetting, on bool) error
}

// nextState is what a toggle moves to. A partial setting goes to on first and
// never returns to partial.
func nextState(s profile.SettingState) profile.SettingState {
	if s == profile.SettingOn {
		return profile.SettingOff
	}
	return profile.SettingOn
}

func (m settingsScreen) Init() tea.Cmd { return nil }

func (m settingsScreen) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	key, ok := msg.(tea.KeyMsg)
	if !ok {
		return m, nil
	}
	switch key.String() {
	case "esc", "q", "ctrl+c":
		return m, tea.Quit
	case "up", "k":
		if m.cursor > 0 {
			m.cursor--
		}
	case "down", "j":
		if m.cursor < len(m.rows)-1 {
			m.cursor++
		}
	case " ", "enter":
		row := &m.rows[m.cursor]
		next := nextState(row.state)
		if err := m.apply(row.setting, next == profile.SettingOn); err != nil {
			m.err = err
			return m, nil
		}
		m.err = nil
		row.state = next
	}
	return m, nil
}

var (
	screenTitle   = lipgloss.NewStyle().Bold(true)
	screenCursor  = lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true)
	screenOn      = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	screenOff     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	screenPartial = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	screenDim     = lipgloss.NewStyle().Faint(true)
	screenErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

func stateLabel(s profile.SettingState) string {
	switch s {
	case profile.SettingOn:
		return screenOn.Render("on")
	case profile.SettingOff:
		return screenOff.Render("off")
	default:
		return screenPartial.Render("partial (hand-edited)")
	}
}

const screenWidth = 44

func (m settingsScreen) View() string {
	var b strings.Builder
	b.WriteString("  " + screenTitle.Render(m.title) + "\n\n")
	labelWidth := 0
	for _, r := range m.rows {
		labelWidth = max(labelWidth, len(r.setting.label))
	}
	for i, r := range m.rows {
		pointer, label := "  ", r.setting.label
		if i == m.cursor {
			pointer, label = screenCursor.Render("❯ "), screenCursor.Render(label)
		}
		pad := strings.Repeat(" ", labelWidth-len(r.setting.label)+4)
		b.WriteString(pointer + label + pad + stateLabel(r.state) + "\n")
	}
	b.WriteString("\n  " + screenDim.Render(strings.Repeat("─", screenWidth)) + "\n")
	desc := lipgloss.NewStyle().Width(screenWidth).Render(m.rows[m.cursor].setting.description)
	for _, line := range strings.Split(desc, "\n") {
		b.WriteString("  " + screenDim.Render(strings.TrimRight(line, " ")) + "\n")
	}
	if m.err != nil {
		b.WriteString("\n  " + screenErr.Render(m.err.Error()) + "\n")
	}
	b.WriteString("\n  " + screenDim.Render("space/enter toggle · ↑/↓ move · esc close") + "\n")
	return b.String()
}
