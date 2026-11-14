package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

func skillsFixture(t *testing.T) (string, string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("HOME", base)
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	t.Setenv("TMPDIR", t.TempDir())
	dir, err := (profile.Store{}).Create("claude", "work", nil)
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(base, "source")
	makeSkill(t, source)
	return dir, source
}
func makeSkill(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "SKILL.md"), []byte("skill"), 0600); err != nil {
		t.Fatal(err)
	}
}

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

func loadRows(t *testing.T, e selectEnv) ([]Row, []string) {
	t.Helper()
	pool, _, err := ScanPools(nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, skipped, err := BuildRows(e.dir, pool)
	if err != nil {
		t.Fatal(err)
	}
	return rows, skipped
}

func rowIndex(t *testing.T, rows []Row, label string) int {
	t.Helper()
	for i, r := range rows {
		if strings.HasPrefix(r.Label, label) {
			return i
		}
	}
	t.Fatalf("no row %q in %+v", label, rows)
	return -1
}

func linkedIndexes(rows []Row) []int {
	var out []int
	for i, r := range rows {
		if r.Linked() {
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
		labels = append(labels, fmt.Sprintf("%s linked=%v", r.Label, r.Linked()))
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
		rows, skipped, err := BuildRows(filepath.Join(profileDir, "skills"), nil)
		if err != nil || len(rows) != 0 || len(skipped) != 0 {
			t.Fatalf("rows=%v skipped=%v err=%v", rows, skipped, err)
		}
	})
	t.Run("relative and mixed pool paths match existing links", func(t *testing.T) {
		e := selectFixture(t)
		t.Chdir(filepath.Dir(e.poolA))
		pool := []PoolSkill{
			{Path: filepath.Join("pool-a", "code-review"), Pool: "pool-a"},
			{Path: filepath.Join(e.poolA, "code-review"), Pool: e.poolA},
		}
		rows, _, err := BuildRows(e.dir, pool)
		if err != nil {
			t.Fatal(err)
		}
		wantSource, err := filepath.EvalSymlinks(filepath.Join(e.poolA, "code-review"))
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for _, r := range rows {
			if r.Name == "code-review" {
				count++
				if !r.Linked() || r.Label != "code-review  pool-a" || r.Source != wantSource {
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
		pool, _, err := ScanPools(func(err error) { warnings = append(warnings, err.Error()) })
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
		if _, _, err := ScanPools(nil); err == nil {
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
			plan, err := PlanChanges(rows, tc.selected)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("err = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			names := func(rs []Row) []string {
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
