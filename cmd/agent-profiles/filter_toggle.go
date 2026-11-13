package main

import (
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// filterToggle wraps a form so space toggles the highlighted option even
// while the filter input has focus. huh sends every key to the filter while
// filtering, so space would be typed into the query instead. Skill names have
// no spaces, so nothing is lost by not typing them.
type filterToggle struct{ form *huh.Form }

func (m filterToggle) Init() tea.Cmd { return m.form.Init() }
func (m filterToggle) View() string  { return m.form.View() }

func (m filterToggle) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeySpace {
		if f, ok := m.form.GetFocusedField().(interface{ GetFiltering() bool }); ok && f.GetFiltering() {
			// esc leaves filter input and keeps the query; then space toggles.
			_, c1 := m.form.Update(tea.KeyMsg{Type: tea.KeyEsc})
			_, c2 := m.form.Update(msg)
			return m, tea.Batch(c1, c2)
		}
	}
	_, cmd := m.form.Update(msg)
	return m, cmd
}

// runFilterToggle runs form like huh.Form.Run, with the space fix above.
func runFilterToggle(form *huh.Form, out io.Writer) error {
	form.SubmitCmd = tea.Quit
	form.CancelCmd = tea.Interrupt
	_, err := tea.NewProgram(filterToggle{form}, tea.WithOutput(out)).Run()
	if form.State == huh.StateAborted || errors.Is(err, tea.ErrInterrupted) {
		return huh.ErrUserAborted
	}
	if err != nil {
		return fmt.Errorf("huh: %w", err)
	}
	return nil
}
