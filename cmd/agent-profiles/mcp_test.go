package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/mcp"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// init keeps every test in this package, including the update tests, away from the
// real MCP pool and from mcporter. Tests that need them call mcpTestEnv.
func init() {
	mcpPoolPath = func() (string, error) { return "/nonexistent-ap-test/mcp.json", nil }
	mcpAvailable = func() string { return "mcporter not found (test default)" }
}

var (
	mcpTestAtlassian = mcp.Server{Type: "http", URL: "https://mcp.atlassian.com/v1/mcp", Auth: "oauth"}
	mcpTestJira      = mcp.Server{Type: "http", URL: "https://jira.example.com/mcp", Auth: "oauth"}
	mcpTestDocs      = mcp.Server{Command: "npx", Args: []string{"-y", "docs-mcp"}}
	mcpTestSentry    = mcp.Server{Type: "sse", URL: "https://mcp.sentry.dev/sse"}
)

// mcpTestEnv isolates HOME, XDG dirs and the pool under a fresh temp dir and sets
// what mcporter availability reports (empty means available). It returns the pool
// path and ap's mcporter config path.
func mcpTestEnv(t *testing.T, unavailable string) (poolPath, mcporterPath string) {
	t.Helper()
	base := t.TempDir()
	t.Setenv("HOME", base)
	t.Setenv("XDG_DATA_HOME", filepath.Join(base, "data"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(base, "config"))
	poolPath = filepath.Join(base, "config", "agent-profiles", "mcp.json")
	oldPool, oldAvail := mcpPoolPath, mcpAvailable
	mcpPoolPath = func() (string, error) { return poolPath, nil }
	mcpAvailable = func() string { return unavailable }
	t.Cleanup(func() { mcpPoolPath, mcpAvailable = oldPool, oldAvail })
	return poolPath, filepath.Join(base, "data", "agent-profiles", "mcporter.json")
}

func mcpTestPool(t *testing.T, path string, servers map[string]mcp.Server) {
	t.Helper()
	if err := mcp.Save(path, mcp.Pool{Servers: servers}); err != nil {
		t.Fatal(err)
	}
}

func mcpTestRun(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(args, &out, &out); err != nil {
		t.Fatalf("%v: %v", args, err)
	}
	return out.String()
}

func mcpTestProfile(t *testing.T, agentName, name string) string {
	t.Helper()
	dir, err := (profile.Store{}).Resolve(agentName, name)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

// mcpTestEntry returns the mcpServers[name] object of an Agent config file, or nil.
func mcpTestEntry(t *testing.T, path, name string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top struct {
		McpServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatal(err)
	}
	return top.McpServers[name]
}

// mcpTestOAuth is the Managed entry for an OAuth Pool server, without the Claude-only type.
func mcpTestOAuth(mcporterPath string) map[string]any {
	return map[string]any{
		"command": "mcporter",
		"args":    []any{"serve", "--stdio", "--servers", "atlassian"},
		"env": map[string]any{
			"MCPORTER_CONFIG":           mcporterPath,
			"MCPORTER_OAUTH_NO_BROWSER": "1",
		},
	}
}

func TestMCPSyncS3OAuthServerReachesEveryAgent(t *testing.T) {
	poolPath, mcporterPath := mcpTestEnv(t, "")
	mcpTestPool(t, poolPath, map[string]mcp.Server{"atlassian": mcpTestAtlassian})
	mcpTestRun(t, "create", "claude", "work", "--init")
	mcpTestRun(t, "create", "pi", "work", "--init")

	got := mcpTestRun(t, "mcp", "sync")
	if want := "mcporter: updated\nclaude/work: + atlassian\npi/work: + atlassian\n"; got != want {
		t.Fatalf("output = %q; want %q", got, want)
	}

	claudeWant := mcpTestOAuth(mcporterPath)
	claudeWant["type"] = "stdio"
	if got := mcpTestEntry(t, filepath.Join(mcpTestProfile(t, "claude", "work"), ".claude.json"), "atlassian"); !reflect.DeepEqual(got, claudeWant) {
		t.Fatalf("claude entry = %v; want %v", got, claudeWant)
	}
	if got := mcpTestEntry(t, filepath.Join(mcpTestProfile(t, "pi", "work"), "mcp.json"), "atlassian"); !reflect.DeepEqual(got, mcpTestOAuth(mcporterPath)) {
		t.Fatalf("pi entry = %v; want %v", got, mcpTestOAuth(mcporterPath))
	}

	data, err := os.ReadFile(mcporterPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]map[string]map[string]string
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	wantCfg := map[string]map[string]string{"atlassian": {"baseUrl": mcpTestAtlassian.URL, "lifecycle": "keep-alive"}}
	if !reflect.DeepEqual(cfg["mcpServers"], wantCfg) {
		t.Fatalf("mcporter config = %v; want %v", cfg["mcpServers"], wantCfg)
	}

	if got := mcpTestRun(t, "mcp", "sync"); got != "claude/work: unchanged\npi/work: unchanged\n" {
		t.Fatalf("second sync output = %q", got)
	}
}

func TestMCPSyncS8NewProfileGetsOnlyTheAddedServers(t *testing.T) {
	poolPath, _ := mcpTestEnv(t, "")
	mcpTestPool(t, poolPath, map[string]mcp.Server{"atlassian": mcpTestAtlassian})
	mcpTestRun(t, "create", "claude", "work", "--init")
	mcpTestRun(t, "create", "pi", "work", "--init")
	mcpTestRun(t, "mcp", "sync")

	mcpTestRun(t, "create", "claude", "personal", "--init")
	got := mcpTestRun(t, "mcp", "sync")
	if want := "claude/personal: + atlassian\nclaude/work: unchanged\npi/work: unchanged\n"; got != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
}

func TestMCPSyncS18MissingMcporterSkipsOAuthOnly(t *testing.T) {
	poolPath, mcporterPath := mcpTestEnv(t, "")
	mcpTestPool(t, poolPath, map[string]mcp.Server{"atlassian": mcpTestAtlassian})
	mcpTestRun(t, "create", "claude", "work", "--init")
	mcpTestRun(t, "create", "pi", "work", "--init")
	mcpTestRun(t, "mcp", "sync")
	before, err := os.ReadFile(mcporterPath)
	if err != nil {
		t.Fatal(err)
	}

	// mcporter goes missing. The pool gains a direct server and a new OAuth server.
	reason := "mcporter not found (needs mcporter >= 0.17.0 and Node >= 24)"
	mcpAvailable = func() string { return reason }
	mcpTestPool(t, poolPath, map[string]mcp.Server{
		"atlassian": mcpTestAtlassian,
		"docs":      mcpTestDocs,
		"jira":      mcpTestJira,
	})
	got := mcpTestRun(t, "mcp", "sync")
	skip := "mcporter unavailable: " + reason
	want := "claude/work: ! atlassian " + skip + "\n" +
		"claude/work: + docs\n" +
		"claude/work: ! jira " + skip + "\n" +
		"pi/work: ! atlassian " + skip + "\n" +
		"pi/work: + docs\n" +
		"pi/work: ! jira " + skip + "\n"
	if got != want {
		t.Fatalf("output = %q; want %q", got, want)
	}

	if !mcpTestHas(t, filepath.Join(mcpTestProfile(t, "claude", "work"), ".claude.json"), "atlassian") ||
		!mcpTestHas(t, filepath.Join(mcpTestProfile(t, "pi", "work"), "mcp.json"), "atlassian") {
		t.Fatal("a Profile lost atlassian while mcporter was unavailable")
	}
	if !mcpTestHas(t, filepath.Join(mcpTestProfile(t, "claude", "work"), ".claude.json"), "docs") {
		t.Fatal("direct server docs was not synced")
	}
	after, err := os.ReadFile(mcporterPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("mcporter config changed while unavailable:\n%s", after)
	}
}

func TestMCPSyncS19ServerOneAgentCannotRun(t *testing.T) {
	poolPath, _ := mcpTestEnv(t, "")
	mcpTestPool(t, poolPath, map[string]mcp.Server{"atlassian": mcpTestAtlassian, "sentry": mcpTestSentry})
	mcpTestRun(t, "create", "claude", "work", "--init")
	mcpTestRun(t, "create", "pi", "work", "--init")

	got := mcpTestRun(t, "mcp", "sync")
	want := "mcporter: updated\n" +
		"claude/work: + atlassian\n" +
		"claude/work: + sentry\n" +
		"pi/work: + atlassian\n" +
		"pi/work: ! sentry SSE not supported by pi\n"
	if got != want {
		t.Fatalf("output = %q; want %q", got, want)
	}
	if e := mcpTestEntry(t, filepath.Join(mcpTestProfile(t, "claude", "work"), ".claude.json"), "sentry"); e["type"] != "sse" || e["url"] != mcpTestSentry.URL {
		t.Fatalf("claude sentry entry = %v", e)
	}
	if mcpTestHas(t, filepath.Join(mcpTestProfile(t, "pi", "work"), "mcp.json"), "sentry") {
		t.Fatal("pi got sentry")
	}
}

func TestUpdateAddsMCPServersToClaudeOnly(t *testing.T) {
	poolPath, mcporterPath := mcpTestEnv(t, "")
	mcpTestPool(t, poolPath, map[string]mcp.Server{"atlassian": mcpTestAtlassian})
	mcpTestRun(t, "create", "claude", "work", "--init")
	mcpTestRun(t, "create", "pi", "work", "--init")

	got := mcpTestRun(t, "update", "claude", "--all")
	if !strings.Contains(got, "claude/work: + atlassian\n") {
		t.Fatalf("update output = %q; want the MCP line for claude/work", got)
	}
	if strings.Contains(got, "pi/") {
		t.Fatalf("update touched Pi: %q", got)
	}
	if got := mcpTestEntry(t, filepath.Join(mcpTestProfile(t, "claude", "work"), ".claude.json"), "atlassian"); !reflect.DeepEqual(got, func() map[string]any {
		m := mcpTestOAuth(mcporterPath)
		m["type"] = "stdio"
		return m
	}()) {
		t.Fatalf("claude entry = %v", got)
	}
	if _, err := os.Stat(filepath.Join(mcpTestProfile(t, "pi", "work"), "mcp.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pi mcp.json was written by update: %v", err)
	}
}

func TestMCPUsageAndHelp(t *testing.T) {
	want := "usage: ap mcp sync\nRun 'ap help mcp' for details."
	for _, args := range [][]string{{"mcp"}, {"mcp", "list"}} {
		var out bytes.Buffer
		err := run(args, &out, &out)
		if err == nil || err.Error() != want {
			t.Fatalf("%v: err = %v; want %q", args, err, want)
		}
	}
	if top := mcpTestRun(t, "help"); !strings.Contains(top, "\nMCP\n") || !strings.Contains(top, "mcp sync") {
		t.Fatalf("top help has no MCP group: %s", top)
	}
}

// mcpTestHas reports whether the Agent config at path lists name under mcpServers.
func mcpTestHas(t *testing.T, path, name string) bool {
	t.Helper()
	return mcpTestEntry(t, path, name) != nil
}
