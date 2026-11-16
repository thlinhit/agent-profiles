package mcp

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMcporterConfigPath(t *testing.T) {
	t.Run("XDG_DATA_HOME", func(t *testing.T) {
		dir := t.TempDir()
		t.Setenv("XDG_DATA_HOME", dir)
		got, err := McporterConfigPath()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(dir, "agent-profiles", "mcporter.json")
		if got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
	t.Run("falls back to ~/.local/share when unset or empty", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("XDG_DATA_HOME", "")
		got, err := McporterConfigPath()
		if err != nil {
			t.Fatal(err)
		}
		want := filepath.Join(home, ".local", "share", "agent-profiles", "mcporter.json")
		if got != want {
			t.Fatalf("got %s, want %s", got, want)
		}
	})
	t.Run("refuses a relative XDG_DATA_HOME", func(t *testing.T) {
		t.Setenv("XDG_DATA_HOME", "relative/dir")
		if _, err := McporterConfigPath(); err == nil {
			t.Fatal("want error for relative XDG_DATA_HOME")
		}
	})
}

const oauthOnlyConfig = "{\n  \"mcpServers\": {\n    \"atlassian\": {\n      \"baseUrl\": \"https://mcp.atlassian.com/v1/mcp\",\n      \"lifecycle\": \"keep-alive\"\n    },\n    \"jira\": {\n      \"baseUrl\": \"https://jira.example/mcp\",\n      \"lifecycle\": \"keep-alive\"\n    }\n  },\n  \"imports\": []\n}\n"

const emptyConfig = "{\n  \"mcpServers\": {},\n  \"imports\": []\n}\n"

func mixedPool() Pool {
	return Pool{Servers: map[string]Server{
		"jira":      {Type: "http", URL: "https://jira.example/mcp", Auth: "oauth"},
		"local":     {Command: "npx", Args: []string{"-y", "some-server"}},
		"atlassian": {Type: "http", URL: "https://mcp.atlassian.com/v1/mcp", Auth: "oauth"},
		"plain":     {Type: "http", URL: "https://plain.example/mcp"},
	}}
}

func TestWriteMcporterConfigMixedPool(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agent-profiles", "mcporter.json")
	changed, err := WriteMcporterConfig(path, mixedPool())
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first write should report changed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != oauthOnlyConfig {
		t.Fatalf("content mismatch:\n%s\nwant:\n%s", data, oauthOnlyConfig)
	}
}

func TestWriteMcporterConfigRemovalLeavesNoEntry(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcporter.json")
	if _, err := WriteMcporterConfig(path, mixedPool()); err != nil {
		t.Fatal(err)
	}
	pool := mixedPool()
	delete(pool.Servers, "jira")
	delete(pool.Servers, "atlassian")
	changed, err := WriteMcporterConfig(path, pool)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("removing servers should report changed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != emptyConfig {
		t.Fatalf("got %q, want %q", data, emptyConfig)
	}
	if strings.Contains(string(data), "jira") || strings.Contains(string(data), "atlassian") {
		t.Fatalf("removed server still present: %s", data)
	}
}

func TestWriteMcporterConfigNoOAuthServers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcporter.json")
	changed, err := WriteMcporterConfig(path, Pool{Servers: map[string]Server{}})
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("first write should report changed")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != emptyConfig {
		t.Fatalf("got %q, want %q", data, emptyConfig)
	}
}

func TestWriteMcporterConfigURLChangeRewrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcporter.json")
	pool := mixedPool()
	if _, err := WriteMcporterConfig(path, pool); err != nil {
		t.Fatal(err)
	}
	s := pool.Servers["jira"]
	s.URL = "https://jira2.example/mcp"
	pool.Servers["jira"] = s
	changed, err := WriteMcporterConfig(path, pool)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("URL change should report changed")
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), `"baseUrl": "https://jira2.example/mcp"`) {
		t.Fatalf("new URL not written: %s", data)
	}
}

func TestWriteMcporterConfigIdenticalBytesNotRewritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcporter.json")
	if _, err := WriteMcporterConfig(path, mixedPool()); err != nil {
		t.Fatal(err)
	}
	old := time.Date(2001, time.January, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	changed, err := WriteMcporterConfig(path, mixedPool())
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("identical bytes should report unchanged")
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ModTime().Equal(old) {
		t.Fatalf("mtime changed to %v, want %v", st.ModTime(), old)
	}
}

func TestWriteMcporterConfigModes(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "agent-profiles")
	path := filepath.Join(dir, "mcporter.json")
	if _, err := WriteMcporterConfig(path, mixedPool()); err != nil {
		t.Fatal(err)
	}
	fst, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := fst.Mode().Perm(); got != 0o600 {
		t.Fatalf("file mode %o, want 600", got)
	}
	dst, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dst.Mode().Perm(); got != 0o700 {
		t.Fatalf("dir mode %o, want 700", got)
	}
}

// fakeCmd is a canned result for one command line, such as "node --version".
type fakeCmd struct {
	out string
	err error
}

// fakeTools returns lookPath and run fakes. Any command not in cmds fails the test.
func fakeTools(t *testing.T, onPath bool, cmds map[string]fakeCmd) (func(string) (string, error), func(string, ...string) (string, error)) {
	t.Helper()
	lookPath := func(name string) (string, error) {
		if !onPath {
			return "", errors.New("not found")
		}
		return "/fake/bin/" + name, nil
	}
	run := func(name string, args ...string) (string, error) {
		key := strings.Join(append([]string{name}, args...), " ")
		c, ok := cmds[key]
		if !ok {
			t.Errorf("unexpected command %q", key)
			return "", errors.New("unexpected")
		}
		return c.out, c.err
	}
	return lookPath, run
}

func TestAvailable(t *testing.T) {
	const unknown = "mcporter version unknown (needs mcporter >= 0.17.0 and Node >= 24)"
	ok := func(out string) fakeCmd { return fakeCmd{out: out} }
	fail := fakeCmd{err: errors.New("exec failed")}
	cases := []struct {
		name   string
		onPath bool
		cmds   map[string]fakeCmd
		want   string
	}{
		{
			name:   "usable",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.17.0\n"), "node --version": ok("v24.0.0\n")},
			want:   "",
		},
		{
			name:   "newer versions with v prefix and trailing text",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("mcporter v0.18.2 (build abc)"), "node --version": ok("v25.1.0 ")},
			want:   "",
		},
		{
			name:   "minor compared numerically, not as text",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.100.0"), "node --version": ok("v24.0.0")},
			want:   "",
		},
		{
			name:   "mcporter not on PATH",
			onPath: false,
			cmds:   nil,
			want:   "mcporter not found (needs mcporter >= 0.17.0 and Node >= 24)",
		},
		{
			name:   "mcporter 0.8.1",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.8.1")},
			want:   "mcporter 0.8.1 is too old (needs >= 0.17.0)",
		},
		{
			name:   "mcporter just below minimum",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("v0.16.9")},
			want:   "mcporter 0.16.9 is too old (needs >= 0.17.0)",
		},
		{
			name:   "mcporter 0.9.0 is below 0.17.0 numerically",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.9.0")},
			want:   "mcporter 0.9.0 is too old (needs >= 0.17.0)",
		},
		{
			name:   "mcporter --version fails",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": fail},
			want:   unknown,
		},
		{
			name:   "mcporter --version has no x.y.z",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("unknown")},
			want:   unknown,
		},
		{
			name:   "mcporter version with only two parts",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.17")},
			want:   unknown,
		},
		{
			name:   "node v22.1.0",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.17.0"), "node --version": ok("v22.1.0")},
			want:   "node v22.1.0 is too old (needs >= 24)",
		},
		{
			name:   "node 23 is too old",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.17.0"), "node --version": ok("v23.11.0")},
			want:   "node v23.11.0 is too old (needs >= 24)",
		},
		{
			name:   "node missing",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.17.0"), "node --version": fail},
			want:   "node not found (needs Node >= 24)",
		},
		{
			name:   "node version has no x.y.z",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.17.0"), "node --version": ok("v24")},
			want:   "node version unknown (needs Node >= 24)",
		},
		{
			name:   "mcporter too old short-circuits before node",
			onPath: true,
			cmds:   map[string]fakeCmd{"mcporter --version": ok("0.8.1")},
			want:   "mcporter 0.8.1 is too old (needs >= 0.17.0)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lookPath, run := fakeTools(t, tc.onPath, tc.cmds)
			if got := Available(lookPath, run); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in   string
		want [3]int
		ok   bool
	}{
		{"0.17.0", [3]int{0, 17, 0}, true},
		{"v24.0.0\n", [3]int{24, 0, 0}, true},
		{"mcporter 1.2.3 (dev)", [3]int{1, 2, 3}, true},
		{"0.17.0-beta.1", [3]int{0, 17, 0}, true},
		{"", [3]int{}, false},
		{"v24", [3]int{}, false},
		{"0.17", [3]int{}, false},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprintf("%q", tc.in), func(t *testing.T) {
			got, ok := parseVersion(tc.in)
			if ok != tc.ok || (ok && got != tc.want) {
				t.Fatalf("parseVersion(%q) = %v, %v; want %v, %v", tc.in, got, ok, tc.want, tc.ok)
			}
		})
	}
}
