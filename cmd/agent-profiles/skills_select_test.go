package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// selectFixture builds two pools and a claude/work profile with:
// code-review (pool A), review (alias of code-review), old-tool (external),
// stale (broken) and notes/ (a real directory).
type selectEnv struct {
	dir, poolA, poolB, external string
}

func selectFixture(t *testing.T) selectEnv {
	t.Helper()
	profileDir, external := skillsFixture(t)
	base := filepath.Dir(external)
	e := selectEnv{
		dir:      filepath.Join(profileDir, "skills"),
		poolA:    filepath.Join(base, "pool-a"),
		poolB:    filepath.Join(base, "pool-b"),
		external: external,
	}
	for _, p := range []string{
		filepath.Join(e.poolA, "code-review"),
		filepath.Join(e.poolA, "review-pr"),
		filepath.Join(e.poolA, "notes"),
		filepath.Join(e.poolB, "code-review"),
	} {
		makeSkill(t, p)
	}
	cfg := filepath.Join(base, "config", "agent-profiles")
	if err := os.MkdirAll(cfg, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg, "config.toml"), []byte(fmt.Sprintf("skill_pools = [%q, %q]\n", e.poolA, e.poolB)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(e.dir, "notes"), 0o700); err != nil {
		t.Fatal(err)
	}
	links := map[string]string{
		"code-review": filepath.Join(e.poolA, "code-review"),
		"review":      filepath.Join(e.poolA, "code-review"),
		"old-tool":    external,
		"stale":       filepath.Join(base, "gone"),
	}
	for name, target := range links {
		if err := os.Symlink(target, filepath.Join(e.dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func loadRows(t *testing.T, e selectEnv) ([]selectRow, []string) {
	t.Helper()
	pool, err := scanPools(nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, skipped, err := buildRows(e.dir, pool)
	if err != nil {
		t.Fatal(err)
	}
	return rows, skipped
}

func rowIndex(t *testing.T, rows []selectRow, label string) int {
	t.Helper()
	for i, r := range rows {
		if strings.HasPrefix(r.Label, label) {
			return i
		}
	}
	t.Fatalf("no row %q in %+v", label, rows)
	return -1
}

func linkedIndexes(rows []selectRow) []int {
	var out []int
	for i, r := range rows {
		if r.Linked {
			out = append(out, i)
		}
	}
	return out
}

func without(s []int, x int) []int {
	var out []int
	for _, v := range s {
		if v != x {
			out = append(out, v)
		}
	}
	return out
}

func TestSelectBuildRows(t *testing.T) {
	e := selectFixture(t)
	rows, skipped := loadRows(t, e)
	var labels []string
	for _, r := range rows {
		labels = append(labels, fmt.Sprintf("%s linked=%v", r.Label, r.Linked))
	}
	got := strings.Join(labels, "\n")
	for _, want := range []string{
		"code-review  pool-a linked=true",
		"code-review  pool-b  (name taken) linked=false",
		"review → code-review  (alias) linked=true",
		"old-tool  (external) linked=true",
		"stale  (broken) linked=true",
		"review-pr  pool-a linked=false",
		"notes  pool-a  (name taken) linked=false",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing row %q in:\n%s", want, got)
		}
	}
	if len(skipped) != 1 || skipped[0] != "notes" {
		t.Errorf("skipped = %v", skipped)
	}
	seen := map[string]bool{}
	for _, r := range rows {
		if seen[r.Label] {
			t.Errorf("duplicate label %q", r.Label)
		}
		seen[r.Label] = true
	}
}

func TestSelectBuildRowsEdges(t *testing.T) {
	t.Run("missing skills dir and empty pool", func(t *testing.T) {
		profileDir, _ := skillsFixture(t)
		rows, skipped, err := buildRows(filepath.Join(profileDir, "skills"), nil)
		if err != nil || len(rows) != 0 || len(skipped) != 0 {
			t.Fatalf("rows=%v skipped=%v err=%v", rows, skipped, err)
		}
	})
	t.Run("relative and mixed pool paths match existing links", func(t *testing.T) {
		e := selectFixture(t)
		t.Chdir(filepath.Dir(e.poolA))
		pool := []poolSkill{
			{Path: filepath.Join("pool-a", "code-review"), Pool: "pool-a"},
			{Path: filepath.Join(e.poolA, "code-review"), Pool: e.poolA},
		}
		rows, _, err := buildRows(e.dir, pool)
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, r := range rows {
			if r.Name == "code-review" {
				count++
				if !r.Linked || r.Kind != rowPool {
					t.Errorf("code-review row = %+v, want linked pool row", r)
				}
			}
		}
		if count != 1 {
			t.Errorf("code-review rows = %d, want 1: %+v", count, rows)
		}
	})
	t.Run("unreadable pool is skipped with warning", func(t *testing.T) {
		e := selectFixture(t)
		if err := os.Chmod(e.poolB, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(e.poolB, 0o700) })
		var warnings []string
		pool, err := scanPools(func(err error) { warnings = append(warnings, err.Error()) })
		if err != nil {
			t.Fatal(err)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "pool-b") {
			t.Fatalf("warnings = %v", warnings)
		}
		for _, sk := range pool {
			if strings.HasPrefix(sk.Path, e.poolB) {
				t.Fatalf("unreadable pool contributed %s", sk.Path)
			}
		}
		if _, err := scanPools(nil); err == nil {
			t.Fatal("strict scan should fail on unreadable pool")
		}
	})
}

func TestSelectPlanChanges(t *testing.T) {
	e := selectFixture(t)
	rows, _ := loadRows(t, e)
	linked := linkedIndexes(rows)
	crA := rowIndex(t, rows, "code-review  pool-a")
	crB := rowIndex(t, rows, "code-review  pool-b")
	alias := rowIndex(t, rows, "review →")
	for _, tc := range []struct {
		name     string
		selected []int
		removes  []string
		adds     []string
		err      string
	}{
		{name: "no-op", selected: linked},
		{name: "swap pools", selected: append(without(linked, crA), crB), removes: []string{"code-review"}, adds: []string{"code-review"}},
		{name: "both same name", selected: append(linked, crB), err: "two skills named code-review"},
		{name: "drop alias only", selected: without(linked, alias), removes: []string{"review"}},
		{name: "clear all", selected: nil, removes: []string{"code-review", "old-tool", "review", "stale"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := planChanges(rows, tc.selected)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			names := func(rs []selectRow) []string {
				var out []string
				for _, r := range rs {
					out = append(out, r.Name)
				}
				return out
			}
			if fmt.Sprint(names(plan.Removes)) != fmt.Sprint(tc.removes) && !(len(plan.Removes) == 0 && tc.removes == nil) {
				t.Errorf("removes = %v, want %v", names(plan.Removes), tc.removes)
			}
			if fmt.Sprint(names(plan.Adds)) != fmt.Sprint(tc.adds) && !(len(plan.Adds) == 0 && tc.adds == nil) {
				t.Errorf("adds = %v, want %v", names(plan.Adds), tc.adds)
			}
		})
	}
}

func TestSelectApply(t *testing.T) {
	t.Run("swap keeps alias and relinks", func(t *testing.T) {
		e := selectFixture(t)
		rows, _ := loadRows(t, e)
		sel := append(without(linkedIndexes(rows), rowIndex(t, rows, "code-review  pool-a")), rowIndex(t, rows, "code-review  pool-b"))
		plan, err := planChanges(rows, sel)
		if err != nil {
			t.Fatal(err)
		}
		if err := preValidate(e.dir, plan); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := applyPlan(e.dir, "claude", "work", plan, &out); err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		got, _ := filepath.EvalSymlinks(filepath.Join(e.dir, "code-review"))
		want, _ := filepath.EvalSymlinks(filepath.Join(e.poolB, "code-review"))
		if got != want {
			t.Errorf("code-review -> %s, want %s", got, want)
		}
		if _, err := os.Lstat(filepath.Join(e.dir, "review")); err != nil {
			t.Errorf("alias removed: %v", err)
		}
		if !strings.Contains(out.String(), "✓ - code-review") || !strings.Contains(out.String(), "✓ + code-review") {
			t.Errorf("report:\n%s", out.String())
		}
	})
	t.Run("prevalidate rejects collision with real dir", func(t *testing.T) {
		e := selectFixture(t)
		rows, _ := loadRows(t, e)
		plan, err := planChanges(rows, append(linkedIndexes(rows), rowIndex(t, rows, "notes  pool-a")))
		if err != nil {
			t.Fatal(err)
		}
		err = preValidate(e.dir, plan)
		if err == nil || !strings.Contains(err.Error(), "+ notes") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("prevalidate rejects vanished source", func(t *testing.T) {
		e := selectFixture(t)
		rows, _ := loadRows(t, e)
		plan, err := planChanges(rows, append(linkedIndexes(rows), rowIndex(t, rows, "review-pr")))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(e.poolA, "review-pr")); err != nil {
			t.Fatal(err)
		}
		if err := preValidate(e.dir, plan); err == nil {
			t.Fatal("want error for vanished source")
		}
	})
	t.Run("partial failure continues and reports", func(t *testing.T) {
		e := selectFixture(t)
		rows, _ := loadRows(t, e)
		sel := without(linkedIndexes(rows), rowIndex(t, rows, "old-tool"))
		sel = append(sel, rowIndex(t, rows, "review-pr"))
		plan, err := planChanges(rows, sel)
		if err != nil {
			t.Fatal(err)
		}
		// old-tool changed into a real directory while the picker was open.
		if err := os.Remove(filepath.Join(e.dir, "old-tool")); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(e.dir, "old-tool"), 0o700); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		err = applyPlan(e.dir, "claude", "work", plan, &out)
		if err == nil {
			t.Fatal("want error")
		}
		if !strings.Contains(out.String(), "✗ - old-tool") || !strings.Contains(out.String(), "✓ + review-pr") {
			t.Errorf("report:\n%s", out.String())
		}
		if st, err := os.Lstat(filepath.Join(e.dir, "old-tool")); err != nil || !st.IsDir() {
			t.Errorf("real dir touched: %v", err)
		}
	})
	t.Run("link replaced while open is left untouched", func(t *testing.T) {
		e := selectFixture(t)
		rows, _ := loadRows(t, e)
		plan, err := planChanges(rows, without(linkedIndexes(rows), rowIndex(t, rows, "old-tool")))
		if err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(e.dir, "old-tool")
		replacement := filepath.Join(e.poolA, "review-pr")
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(replacement, link); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := applyPlan(e.dir, "claude", "work", plan, &out); err == nil {
			t.Fatal("want error")
		}
		if got, _ := os.Readlink(link); got != replacement {
			t.Errorf("replaced link removed or changed: %q", got)
		}
		if !strings.Contains(out.String(), "changed since the picker opened") {
			t.Errorf("report:\n%s", out.String())
		}
	})
	t.Run("failed swap prints restore command", func(t *testing.T) {
		e := selectFixture(t)
		rows, _ := loadRows(t, e)
		sel := append(without(linkedIndexes(rows), rowIndex(t, rows, "code-review  pool-a")), rowIndex(t, rows, "code-review  pool-b"))
		plan, err := planChanges(rows, sel)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(filepath.Join(e.poolB, "code-review")); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := applyPlan(e.dir, "claude", "work", plan, &out); err == nil {
			t.Fatal("want error")
		}
		oldSource, _ := filepath.EvalSymlinks(filepath.Join(e.poolA, "code-review"))
		if !strings.Contains(out.String(), "restore with: ap skills add claude work "+oldSource+" --name code-review") {
			t.Errorf("report:\n%s", out.String())
		}
	})
}

func TestSelectRequiresTerminal(t *testing.T) {
	selectFixture(t)
	_, err := skillsRun("select", "claude", "work")
	if err == nil || !strings.Contains(err.Error(), "requires a terminal") {
		t.Fatalf("err = %v", err)
	}
}

// drive feeds a key into the form and runs resulting commands synchronously,
// following huh's own test helper.
func drive(f *huh.Form, msg tea.Msg) {
	_, cmd := f.Update(msg)
	runCmds(f, cmd, 0)
}

func runCmds(f *huh.Form, cmd tea.Cmd, depth int) {
	if cmd == nil || depth > 20 {
		return
	}
	// Timer commands (cursor blink) sleep before returning; drop them.
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var got tea.Msg
	select {
	case got = <-done:
	case <-time.After(50 * time.Millisecond):
		return
	}
	switch msg := got.(type) {
	case tea.BatchMsg:
		for _, c := range msg {
			runCmds(f, c, depth+1)
		}
	case nil:
	default:
		_, next := f.Update(msg)
		runCmds(f, next, depth+1)
	}
}

func key(k tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: k} }
func runes(s string) tea.KeyMsg    { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)} }

func newTestForm(t *testing.T) (*huh.Form, []selectRow, *[]int, *bool) {
	t.Helper()
	e := selectFixture(t)
	rows, _ := loadRows(t, e)
	selected := linkedIndexes(rows)
	ok := true
	f := buildSelectForm("test", rows, nil, &selected, &ok)
	runCmds(f, f.Init(), 0)
	return f, rows, &selected, &ok
}

func TestSelectFormToggleAndConfirm(t *testing.T) {
	f, rows, selected, ok := newTestForm(t)
	// Row 0 is code-review (pool-a, linked): untick it, then continue.
	drive(f, key(tea.KeySpace))
	drive(f, key(tea.KeyEnter))
	if f.State != huh.StateNormal {
		t.Fatalf("state after list = %v", f.State)
	}
	plan, err := planChanges(rows, *selected)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Removes) != 1 || plan.Removes[0].Label != rows[0].Label {
		t.Fatalf("plan = %+v", plan)
	}
	if !strings.Contains(f.View(), "- "+rows[0].Name) {
		t.Errorf("confirm diff missing removal:\n%s", f.View())
	}
	drive(f, runes("n"))
	if f.State != huh.StateCompleted || *ok {
		t.Fatalf("state=%v ok=%v, want completed with No", f.State, *ok)
	}
}

func TestSelectFormFilterAndAbort(t *testing.T) {
	f, _, _, _ := newTestForm(t)
	drive(f, runes("/"))
	for _, r := range "pr" {
		drive(f, runes(string(r)))
	}
	if view := f.View(); !strings.Contains(view, "review-pr") || strings.Contains(view, "old-tool") {
		t.Errorf("filtered view:\n%s", view)
	}
	drive(f, key(tea.KeyCtrlC))
	if f.State != huh.StateAborted {
		t.Fatalf("state = %v, want aborted", f.State)
	}
}

func TestSelectFormBlocksDuplicateNames(t *testing.T) {
	f, rows, selected, _ := newTestForm(t)
	crB := -1
	for i, r := range rows {
		if strings.HasPrefix(r.Label, "code-review  pool-b") {
			crB = i
		}
	}
	for i := 0; i < crB; i++ {
		drive(f, key(tea.KeyDown))
	}
	drive(f, key(tea.KeySpace))
	drive(f, key(tea.KeyEnter))
	if !strings.Contains(f.View(), "two skills named code-review") {
		t.Errorf("want validation error, view:\n%s\nselected=%v", f.View(), *selected)
	}
}
