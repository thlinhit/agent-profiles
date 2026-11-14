package skills

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/thlinhit/agent-profiles/internal/dotfile"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// Row is one picker entry. Identity is the destination link name plus
// the canonical source, so aliases of one source stay distinct.
type Row struct {
	Name   string // destination link name in the profile
	Source string // canonical source path; empty for broken links
	Target string // raw symlink target for existing links
	Label  string
}

func (r Row) Linked() bool { return r.Target != "" }

type Plan struct {
	Removes []Row
	Adds    []Row
}

func (p Plan) Empty() bool { return len(p.Removes) == 0 && len(p.Adds) == 0 }

// BuildRows merges the profile's current links with pool Skills. Links that
// are not symlinks are returned in skipped and never offered for change.
func BuildRows(dir string, pool []PoolSkill) (rows []Row, skipped []string, err error) {
	canonical := map[string]string{}
	for _, sk := range pool {
		real, err := dotfile.Canonical(sk.Path) // hand-edited config may hold relative pools
		if err != nil {
			continue
		}
		if _, dup := canonical[real]; !dup { // overlapping pools: keep first
			canonical[real] = sk.Pool
		}
	}
	taken := map[string]bool{}
	ownLinked := map[string]bool{} // canonical sources linked under their own name

	for link, err := range Links(dir, canonical) {
		if err != nil {
			return nil, nil, err
		}
		taken[link.Name] = true
		if link.Kind == NonLink {
			skipped = append(skipped, link.Name)
			continue
		}
		row := Row{Name: link.Name, Target: link.Target, Source: link.Source}
		switch link.Kind {
		case Broken:
			row.Label = link.Name + "  (broken)"
		case Pool:
			row.Label = link.Name + "  " + filepath.Base(link.Pool)
			ownLinked[link.Source] = true
		case Alias:
			row.Label = link.Name + " → " + filepath.Base(link.Source) + "  (alias)"
		case External:
			row.Label = link.Name + "  (external)"
		}
		rows = append(rows, row)
	}
	for _, real := range slices.Sorted(maps.Keys(canonical)) {
		if ownLinked[real] {
			continue
		}
		name := filepath.Base(real)
		label := name + "  " + filepath.Base(canonical[real])
		if taken[name] {
			label += "  (name taken)"
		}
		rows = append(rows, Row{Name: name, Source: real, Label: label})
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

// PlanChanges diffs the current links against the selected row indexes.
func PlanChanges(rows []Row, selected []int) (Plan, error) {
	sel := map[int]bool{}
	for _, i := range selected {
		sel[i] = true
	}
	var plan Plan
	final := map[string]int{}
	for i, r := range rows {
		switch {
		case r.Linked() && !sel[i]:
			plan.Removes = append(plan.Removes, r)
		case !r.Linked() && sel[i]:
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
		return Plan{}, fmt.Errorf("two skills named %s; pick one", strings.Join(dups, ", "))
	}
	return plan, nil
}

// PreValidate checks every add before anything changes, so predictable
// failures abort with the profile untouched.
func PreValidate(dir string, plan Plan, skillSource func(string) (string, error)) error {
	removed := map[string]bool{}
	for _, r := range plan.Removes {
		removed[r.Name] = true
	}
	if err := checkOptionalDir(dir); err != nil {
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

// ApplyPlan runs removes then adds, best effort. report receives each result and
// the previous source when a failed add can be restored.
func ApplyPlan(dir string, plan Plan, skillSource func(string) (string, error), report func(Row, bool, error, string)) error {
	failed := 0
	removedTarget := map[string]string{}
	for _, r := range plan.Removes {
		err := unchangedLink(dir, r)
		if err == nil {
			err = Remove(dir, r.Name)
		}
		if err != nil {
			failed++
		} else {
			removedTarget[r.Name] = r.Source
		}
		report(r, true, err, "")
	}
	for _, r := range plan.Adds {
		source, err := skillSource(r.Source)
		if err == nil {
			err = Add(dir, source, r.Name)
		}
		restore := ""
		if err != nil {
			failed++
			restore = removedTarget[r.Name]
		}
		report(r, false, err, restore)
	}
	if failed > 0 {
		return fmt.Errorf("%d skill change(s) failed", failed)
	}
	return nil
}

// unchangedLink refuses to remove a link replaced since the picker read it.
func unchangedLink(dir string, r Row) error {
	target, err := os.Readlink(filepath.Join(dir, r.Name))
	if err != nil {
		return err
	}
	if target != r.Target {
		return fmt.Errorf("changed since the picker opened (now -> %s); left untouched", target)
	}
	return nil
}
