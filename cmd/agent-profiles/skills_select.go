package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
	"github.com/thlinhit/agent-profiles/internal/shell"
	"github.com/thlinhit/agent-profiles/internal/skills"
)

// applyPlan keeps change reports and restore hints in the CLI.
func applyPlan(dir, agentName, profileName string, plan skills.Plan, out io.Writer, skillSource func(string) (string, error)) error {
	return skills.ApplyPlan(dir, plan, skillSource, func(r skills.Row, remove bool, err error, restore string) {
		sign := "+"
		if remove {
			sign = "-"
		}
		if err != nil {
			fmt.Fprintf(out, "✗ %s %s: %v\n", sign, r.Name, err)
			if restore != "" {
				args := []string{agentName, profileName, restore, r.Name}
				for i, arg := range args {
					quoted, err := shell.Quote(arg)
					if err != nil {
						quoted = "'<invalid NUL>'"
					}
					args[i] = quoted
				}
				fmt.Fprintf(out, "  restore with: ap skills add %s %s %s --name %s\n", args[0], args[1], args[2], args[3])
			}
			return
		}
		fmt.Fprintf(out, "✓ %s %s\n", sign, r.Name)
	})
}

func describePlan(plan skills.Plan, err error) string {
	if err != nil {
		return err.Error()
	}
	if plan.Empty() {
		return "No changes."
	}
	var b strings.Builder
	for _, r := range plan.Removes {
		fmt.Fprintf(&b, "  - %s\n", r.Name)
	}
	for _, r := range plan.Adds {
		fmt.Fprintf(&b, "  + %s  → %s\n", r.Name, r.Source)
	}
	return strings.TrimRight(b.String(), "\n")
}

// buildSelectForm builds the checklist and confirm pages. selected and ok are
// bound so the confirm diff and the caller see live values.
func buildSelectForm(title string, rows []skills.Row, notes []string, selected *[]int, ok *bool) *huh.Form {
	opts := make([]huh.Option[int], len(rows))
	for i, r := range rows {
		opts[i] = huh.NewOption(r.Label, i)
	}
	desc := "space/x toggle · / filter (esc to finish) · ctrl+a toggle shown · enter continue · ctrl+c abort"
	if len(notes) > 0 {
		desc += "\n" + strings.Join(notes, "\n")
	}
	height := len(rows) + 4
	if height > 20 {
		height = 20
	}
	list := huh.NewMultiSelect[int]().
		Title(title).
		Description(desc).
		Options(opts...).
		Value(selected).
		Filterable(true).
		Height(height).
		Validate(func(v []int) error {
			_, err := skills.PlanChanges(rows, v)
			return err
		})
	confirm := huh.NewConfirm().
		Title("Apply these changes?").
		DescriptionFunc(func() string {
			return describePlan(skills.PlanChanges(rows, *selected))
		}, selected).
		Affirmative("Yes").
		Negative("No (back to list)").
		Value(ok)
	return huh.NewForm(huh.NewGroup(list), huh.NewGroup(confirm))
}

func runSkillSelect(s profile.Store, args []string, out io.Writer) error {
	if len(args) > 2 {
		return errors.New(skillUsage("select"))
	}
	// The ap shell function captures stdout with $(...) for eval, so the UI
	// renders on stderr and only stdin and stderr must be terminals.
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return fmt.Errorf("skills select requires a terminal; use ap skills add/rm non-interactively")
	}
	// lipgloss detects colors from stdout by default. stdout is a pipe here,
	// so it would fall back to no colors; detect from stderr instead.
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
			opts = append(opts, huh.NewOption(a.Name(), a.Name()))
		}
		if err := runPrompt(huh.NewSelect[string]().Title("Select agent").Options(opts...).Value(&agentName)); err != nil {
			return abortErr(err, out)
		}
	}
	a, err := agent.Get(agentName)
	if err != nil {
		return err
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
	profileDir := dir
	dir = filepath.Join(dir, a.SkillsSubdir())

	var notes []string
	pool, _, err := skills.ScanPools(func(err error) { notes = append(notes, "warning: "+err.Error()) })
	if err != nil {
		return err
	}
	rows, skipped, err := skills.BuildRows(dir, pool)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("no skills to select; add a pool with skills pool add <path>")
	}
	for _, name := range skipped {
		notes = append(notes, "skipped: "+name+" (not managed by ap)")
	}
	linked := 0
	var selected []int
	for i, r := range rows {
		if r.Linked() {
			linked++
			selected = append(selected, i)
		}
	}
	title := fmt.Sprintf("Skills for %s / %s  (%d linked · %d in pools)", a.Name(), profileName, linked, len(pool))

	skillSource := skills.NewSource()
	for {
		ok := true
		if err := runFilterToggle(buildSelectForm(title, rows, notes, &selected, &ok), os.Stderr); err != nil {
			return abortErr(err, out)
		}
		if !ok {
			continue // No: back to the list, selections kept
		}
		plan, err := skills.PlanChanges(rows, selected)
		if err != nil {
			return err // unreachable: list validation blocks duplicates
		}
		if plan.Empty() {
			_, err := fmt.Fprintln(out, "No changes.")
			return err
		}
		if err := skills.PreValidate(dir, plan, skillSource); err != nil {
			return fmt.Errorf("no changes made:\n%w", err)
		}
		applyErr := applyPlan(dir, a.Name(), profileName, plan, out, skillSource)
		return errors.Join(applyErr, skills.SyncDirs(a, profileName, profileDir))
	}
}

func runPrompt(f huh.Field) error {
	return huh.NewForm(huh.NewGroup(f)).WithOutput(os.Stderr).Run()
}

func abortErr(err error, out io.Writer) error {
	if errors.Is(err, huh.ErrUserAborted) {
		_, err := fmt.Fprintln(out, "Cancelled, no changes made.")
		return err
	}
	return err
}
