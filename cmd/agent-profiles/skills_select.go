package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

type rowKind int

const (
	rowPool     rowKind = iota // pool skill linked (or linkable) under its own name
	rowAlias                   // link to a pool skill under a different name
	rowExternal                // link to a skill outside the pools
	rowBroken                  // link whose target no longer exists
)

// selectRow is one picker entry. Identity is the destination link name plus
// the canonical source, so aliases of one source stay distinct.
type selectRow struct {
	Name   string // destination link name in the profile
	Source string // canonical source path; empty for broken links
	Target string // raw symlink target for existing links
	Label  string
	Linked bool
	Kind   rowKind
}

type skillPlan struct {
	Removes []selectRow
	Adds    []selectRow
}

func (p skillPlan) empty() bool { return len(p.Removes) == 0 && len(p.Adds) == 0 }

// buildRows merges the profile's current links with pool skills. Entries that
// are not symlinks are returned in skipped and never offered for change.
func buildRows(dir string, pool []poolSkill) (rows []selectRow, skipped []string, err error) {
	canonical := map[string]poolSkill{}
	for _, sk := range pool {
		abs, err := filepath.Abs(sk.Path) // hand-edited config may hold relative pools
		if err != nil {
			continue
		}
		real, err := filepath.EvalSymlinks(abs)
		if err != nil {
			continue
		}
		if _, dup := canonical[real]; !dup { // overlapping pools: keep first
			canonical[real] = poolSkill{Path: real, Pool: sk.Pool}
		}
	}
	linkedNames := map[string]bool{}
	ownLinked := map[string]bool{} // canonical sources linked under their own name

	entries, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		return nil, nil, err
	}
	if err == nil {
		if err := checkSkillsDir(dir); err != nil {
			return nil, nil, err
		}
	}
	for _, e := range entries {
		path := filepath.Join(dir, e.Name())
		if e.Type()&os.ModeSymlink == 0 {
			skipped = append(skipped, e.Name())
			linkedNames[e.Name()] = true // name is occupied
			continue
		}
		target, err := os.Readlink(path)
		if err != nil {
			return nil, nil, err
		}
		linkedNames[e.Name()] = true
		row := selectRow{Name: e.Name(), Target: target, Linked: true}
		real, err := filepath.EvalSymlinks(path)
		switch {
		case os.IsNotExist(err):
			row.Kind, row.Label = rowBroken, e.Name()+"  (broken)"
		case err != nil:
			return nil, nil, err
		default:
			row.Source = real
			if sk, ok := canonical[real]; ok {
				if filepath.Base(real) == e.Name() {
					row.Kind, row.Label = rowPool, e.Name()+"  "+filepath.Base(sk.Pool)
					ownLinked[real] = true
				} else {
					row.Kind, row.Label = rowAlias, e.Name()+" → "+filepath.Base(real)+"  (alias)"
				}
			} else {
				row.Kind, row.Label = rowExternal, e.Name()+"  (external)"
			}
		}
		rows = append(rows, row)
	}
	var reals []string
	for real := range canonical {
		reals = append(reals, real)
	}
	sort.Strings(reals)
	for _, real := range reals {
		if ownLinked[real] {
			continue
		}
		name := filepath.Base(real)
		label := name + "  " + filepath.Base(canonical[real].Pool)
		if linkedNames[name] {
			label += "  (name taken)"
		}
		rows = append(rows, selectRow{Name: name, Source: real, Label: label, Kind: rowPool})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	// huh toggles options by label, so labels must be unique.
	seen := map[string]int{}
	for _, r := range rows {
		seen[r.Label]++
	}
	for i, r := range rows {
		if seen[r.Label] > 1 {
			rows[i].Label = r.Label + "  " + r.Source
		}
	}
	return rows, skipped, nil
}

// planChanges diffs the current links against the selected row indexes.
func planChanges(rows []selectRow, selected []int) (skillPlan, error) {
	sel := map[int]bool{}
	for _, i := range selected {
		sel[i] = true
	}
	var plan skillPlan
	final := map[string]int{}
	for i, r := range rows {
		switch {
		case r.Linked && !sel[i]:
			plan.Removes = append(plan.Removes, r)
		case !r.Linked && sel[i]:
			plan.Adds = append(plan.Adds, r)
		}
		if sel[i] {
			final[r.Name]++
		}
	}
	var dups []string
	for name, n := range final {
		if n > 1 {
			dups = append(dups, name)
		}
	}
	if len(dups) > 0 {
		sort.Strings(dups)
		return skillPlan{}, fmt.Errorf("two skills named %s; pick one", strings.Join(dups, ", "))
	}
	return plan, nil
}

// preValidate checks every add before anything changes, so predictable
// failures abort with the profile untouched.
func preValidate(dir string, plan skillPlan) error {
	removed := map[string]bool{}
	for _, r := range plan.Removes {
		removed[r.Name] = true
	}
	if _, err := os.Lstat(dir); err == nil {
		if err := checkSkillsDir(dir); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	var errs []error
	for _, r := range plan.Adds {
		if err := profile.ValidateName(r.Name); err != nil {
			errs = append(errs, fmt.Errorf("+ %s: %w", r.Name, err))
			continue
		}
		if _, err := skillSource(r.Source); err != nil {
			errs = append(errs, fmt.Errorf("+ %s: %w", r.Name, err))
			continue
		}
		if !removed[r.Name] {
			if _, err := os.Lstat(filepath.Join(dir, r.Name)); err == nil {
				errs = append(errs, fmt.Errorf("+ %s: already exists (not managed by ap or not selected for removal)", r.Name))
			} else if !os.IsNotExist(err) {
				errs = append(errs, fmt.Errorf("+ %s: %w", r.Name, err))
			}
		}
	}
	return errors.Join(errs...)
}

// applyPlan runs removes then adds, best effort, reporting each item.
func applyPlan(dir, agentName, profileName string, plan skillPlan, out io.Writer) error {
	failed := 0
	removedTarget := map[string]string{}
	for _, r := range plan.Removes {
		err := unchangedLink(dir, r)
		if err == nil {
			err = removeSkill(dir, r.Name)
		}
		if err != nil {
			failed++
			fmt.Fprintf(out, "✗ - %s: %v\n", r.Name, err)
			continue
		}
		removedTarget[r.Name] = r.Source // canonical absolute; empty for broken links
		fmt.Fprintf(out, "✓ - %s\n", r.Name)
	}
	for _, r := range plan.Adds {
		source, err := skillSource(r.Source)
		if err == nil {
			err = addSkill(dir, source, r.Name)
		}
		if err != nil {
			failed++
			fmt.Fprintf(out, "✗ + %s: %v\n", r.Name, err)
			if old, ok := removedTarget[r.Name]; ok && old != "" {
				fmt.Fprintf(out, "  restore with: ap skills add %s %s %s --name %s\n", shellQuote(agentName), shellQuote(profileName), shellQuote(old), shellQuote(r.Name))
			}
			continue
		}
		fmt.Fprintf(out, "✓ + %s\n", r.Name)
	}
	if failed > 0 {
		return fmt.Errorf("%d skill change(s) failed", failed)
	}
	return nil
}

// unchangedLink refuses to remove a link replaced since the picker read it.
func unchangedLink(dir string, r selectRow) error {
	target, err := os.Readlink(filepath.Join(dir, r.Name))
	if err != nil {
		return err
	}
	if target != r.Target {
		return fmt.Errorf("changed since the picker opened (now -> %s); left untouched", target)
	}
	return nil
}

func shellQuote(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_./~") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func describePlan(plan skillPlan, err error) string {
	if err != nil {
		return err.Error()
	}
	if plan.empty() {
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
func buildSelectForm(title string, rows []selectRow, notes []string, selected *[]int, ok *bool) *huh.Form {
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
			_, err := planChanges(rows, v)
			return err
		})
	confirm := huh.NewConfirm().
		Title("Apply these changes?").
		DescriptionFunc(func() string {
			return describePlan(planChanges(rows, *selected))
		}, selected).
		Affirmative("Yes").
		Negative("No (back to list)").
		Value(ok)
	return huh.NewForm(huh.NewGroup(list), huh.NewGroup(confirm))
}

func runSkillSelect(s profile.Store, args []string, out io.Writer) error {
	if len(args) > 2 {
		return fmt.Errorf("usage: skills select [<agent> [<profile>]]")
	}
	// The ap shell function captures stdout with $(...) for eval, so the UI
	// renders on stderr and only stdin and stderr must be terminals.
	if !term.IsTerminal(int(os.Stdin.Fd())) || !term.IsTerminal(int(os.Stderr.Fd())) {
		return fmt.Errorf("skills select requires a terminal; use skills add/rm")
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
	dir = filepath.Join(dir, a.SkillsSubdir())

	var notes []string
	pool, err := scanPools(func(err error) { notes = append(notes, "warning: "+err.Error()) })
	if err != nil {
		return err
	}
	rows, skipped, err := buildRows(dir, pool)
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
		if r.Linked {
			linked++
			selected = append(selected, i)
		}
	}
	title := fmt.Sprintf("Skills for %s / %s  (%d linked · %d in pools)", a.Name(), profileName, linked, len(pool))

	for {
		ok := true
		if err := runFilterToggle(buildSelectForm(title, rows, notes, &selected, &ok), os.Stderr); err != nil {
			return abortErr(err, out)
		}
		plan, err := planChanges(rows, selected)
		if err != nil {
			return err // unreachable: list validation blocks duplicates
		}
		if !ok {
			continue // No: back to the list, selections kept
		}
		if plan.empty() {
			_, err := fmt.Fprintln(out, "No changes.")
			return err
		}
		if err := preValidate(dir, plan); err != nil {
			return fmt.Errorf("no changes made:\n%w", err)
		}
		applyErr := applyPlan(dir, a.Name(), profileName, plan, out)
		return errors.Join(applyErr, syncSkillDirs(s, a, profileName))
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
