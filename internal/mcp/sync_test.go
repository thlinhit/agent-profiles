package mcp_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/thlinhit/agent-profiles/internal/agent/claude"
	"github.com/thlinhit/agent-profiles/internal/mcp"
)

var (
	atlassian = mcp.Server{Type: "http", URL: "https://mcp.atlassian.com/v1/mcp", Auth: "oauth"}
	docs      = mcp.Server{Command: "npx", Args: []string{"-y", "docs-mcp"}}
	proxyOK   = mcp.Proxy{ConfigPath: "/data/agent-profiles/mcporter.json"}
	proxyDown = mcp.Proxy{
		ConfigPath:  "/data/agent-profiles/mcporter.json",
		Unavailable: "mcporter not found (needs mcporter >= 0.17.0 and Node >= 24)",
	}
)

// jsonSetter is a small Agent for these tests. Its config is mcp.json with a
// top-level mcpServers object. It skips SSE servers, as Pi does.
type jsonSetter struct{}

func (jsonSetter) MCPConfigFile(dir string) string { return filepath.Join(dir, "mcp.json") }

func (jsonSetter) Translate(name string, s mcp.Server, proxy mcp.Proxy) (json.RawMessage, string, error) {
	switch {
	case name == "boom":
		return nil, "", errors.New("cannot translate")
	case s.Type == "sse":
		return nil, "SSE not supported by pi", nil
	case s.Auth == "oauth" && proxy.Unavailable != "":
		return nil, "mcporter unavailable: " + proxy.Unavailable, nil
	case s.Auth == "oauth":
		b, err := json.Marshal(mcp.OAuthEntry(name, proxy))
		return b, "", err
	}
	b, err := json.Marshal(s)
	return b, "", err
}

func (jsonSetter) SetServers(current []byte, prev []string, next map[string]json.RawMessage) ([]byte, error) {
	top := map[string]json.RawMessage{}
	if len(current) > 0 {
		if err := json.Unmarshal(current, &top); err != nil {
			return nil, err
		}
	}
	servers := map[string]json.RawMessage{}
	if raw, ok := top["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return nil, err
		}
	}
	want := map[string]json.RawMessage{}
	maps.Copy(want, servers)
	for _, name := range prev {
		if _, keep := next[name]; !keep {
			delete(want, name)
		}
	}
	maps.Copy(want, next)
	before, _ := json.Marshal(servers)
	after, _ := json.Marshal(want)
	if bytes.Equal(before, after) {
		return current, nil
	}
	top["mcpServers"] = after
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// orderSetter is the Claude Adapter, plus a record of whether the backup
// file already existed when SetServers ran, which is before the Agent config is written.
type orderSetter struct {
	claude.Adapter
	dir        string
	backupSeen *bool
}

func (o orderSetter) SetServers(current []byte, prev []string, next map[string]json.RawMessage) ([]byte, error) {
	if _, err := os.Stat(filepath.Join(o.dir, ".ap-mcp-backup.json")); err == nil {
		*o.backupSeen = true
	}
	return o.Adapter.SetServers(current, prev, next)
}

func writeTest(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

func readTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func pool(servers map[string]mcp.Server) mcp.Pool {
	return mcp.Pool{Servers: servers}
}

func syncClaude(t *testing.T, dir string, p mcp.Pool, proxy mcp.Proxy) mcp.Report {
	t.Helper()
	r, err := mcp.Sync(claude.Adapter{}, dir, p, proxy)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	return r
}

func wantLines(t *testing.T, got mcp.Report, want ...string) {
	t.Helper()
	if !slices.Equal(got.Lines, want) {
		t.Fatalf("lines = %q, want %q", got.Lines, want)
	}
}

// mutateServers edits the mcpServers of a JSON config in place, as a hand edit would.
func mutateServers(t *testing.T, path string, fn func(servers map[string]any)) {
	t.Helper()
	var top map[string]any
	if err := json.Unmarshal([]byte(readTest(t, path)), &top); err != nil {
		t.Fatal(err)
	}
	servers, _ := top["mcpServers"].(map[string]any)
	if servers == nil {
		servers = map[string]any{}
		top["mcpServers"] = servers
	}
	fn(servers)
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	writeTest(t, path, string(out)+"\n")
}

// serverNames returns the sorted mcpServers names of a Claude config.
func serverNames(t *testing.T, path string) []string {
	t.Helper()
	var top struct {
		McpServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readTest(t, path)), &top); err != nil {
		t.Fatal(err)
	}
	return slices.Sorted(maps.Keys(top.McpServers))
}

// entry returns the decoded mcpServers entry for name, or nil when it is absent.
func entry(t *testing.T, path, name string) map[string]any {
	t.Helper()
	var top struct {
		McpServers map[string]map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readTest(t, path)), &top); err != nil {
		t.Fatal(err)
	}
	return top.McpServers[name]
}

func TestSyncAddsPoolServersAndWritesLedger(t *testing.T) {
	dir := t.TempDir()
	r := syncClaude(t, dir, pool(map[string]mcp.Server{"atlassian": atlassian, "docs": docs}), proxyOK)
	wantLines(t, r, "+ atlassian", "+ docs")
	if !r.Changed() {
		t.Fatal("Changed = false, want true")
	}
	cfg := filepath.Join(dir, ".claude.json")
	if got := entry(t, cfg, "atlassian")["command"]; got != "mcporter" {
		t.Fatalf("atlassian command = %v, want mcporter", got)
	}
	if got := readTest(t, filepath.Join(dir, ".ap-mcp.json")); got != `["atlassian","docs"]` {
		t.Fatalf("ledger = %s", got)
	}
}

func TestSyncOAuthURLChangeLeavesProfileAlone(t *testing.T) {
	dir := t.TempDir()
	syncClaude(t, dir, pool(map[string]mcp.Server{"atlassian": atlassian}), proxyOK)
	cfg := filepath.Join(dir, ".claude.json")
	before := readTest(t, cfg)

	moved := atlassian
	moved.URL = "https://mcp.atlassian.com/v2/mcp"
	r := syncClaude(t, dir, pool(map[string]mcp.Server{"atlassian": moved}), proxyOK)
	wantLines(t, r)
	if r.Changed() {
		t.Fatal("Changed = true after a URL change to an OAuth server")
	}
	if got := readTest(t, cfg); got != before {
		t.Fatalf("Profile changed:\n%s", got)
	}
}

func TestSyncResetsHandEditToManagedServer(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".claude.json")
	p := pool(map[string]mcp.Server{"docs": docs})
	syncClaude(t, dir, p, proxyOK)
	mutateServers(t, cfg, func(s map[string]any) {
		s["docs"].(map[string]any)["args"] = []any{"hacked"}
	})
	r := syncClaude(t, dir, p, proxyOK)
	wantLines(t, r, "~ docs")
	if got := entry(t, cfg, "docs")["args"]; got == nil || got.([]any)[0] != "-y" {
		t.Fatalf("docs args = %v, want the pool's version", got)
	}
}

func TestSyncReAddsHandDeletedManagedServer(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".claude.json")
	p := pool(map[string]mcp.Server{"docs": docs})
	syncClaude(t, dir, p, proxyOK)
	mutateServers(t, cfg, func(s map[string]any) { delete(s, "docs") })
	wantLines(t, syncClaude(t, dir, p, proxyOK), "+ docs")
	if entry(t, cfg, "docs") == nil {
		t.Fatal("docs not re-added")
	}
}

func TestSyncClashBacksUpHandMadeServerThenAppendsNewDefinitions(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".claude.json")
	backupPath := filepath.Join(dir, ".ap-mcp-backup.json")
	writeTest(t, cfg, `{"mcpServers":{"atlassian":{"type":"http","url":"https://old.example/mcp"}}}`)

	seen := false
	r, err := mcp.Sync(orderSetter{dir: dir, backupSeen: &seen}, dir,
		pool(map[string]mcp.Server{"atlassian": atlassian}), proxyOK)
	if err != nil {
		t.Fatal(err)
	}
	wantLines(t, r, "! atlassian replaced by pool; backup saved")
	if !seen {
		t.Fatal("backup was not written before the Agent config")
	}
	var b struct {
		Version int                          `json:"version"`
		Servers map[string][]json.RawMessage `json:"servers"`
	}
	if err := json.Unmarshal([]byte(readTest(t, backupPath)), &b); err != nil {
		t.Fatal(err)
	}
	if b.Version != 1 || len(b.Servers["atlassian"]) != 1 || !strings.Contains(string(b.Servers["atlassian"][0]), "old.example") {
		t.Fatalf("backup = %+v", b)
	}
	if got := entry(t, cfg, "atlassian")["command"]; got != "mcporter" {
		t.Fatalf("atlassian not replaced: %v", got)
	}

	// The pool drops atlassian, so it is no longer Managed; the user makes a different one by hand.
	syncClaude(t, dir, pool(nil), proxyOK)
	mutateServers(t, cfg, func(s map[string]any) {
		s["atlassian"] = map[string]any{"type": "http", "url": "https://other.example/mcp"}
	})
	wantLines(t, syncClaude(t, dir, pool(map[string]mcp.Server{"atlassian": atlassian}), proxyOK),
		"! atlassian replaced by pool; backup saved")
	b.Servers = nil
	if err := json.Unmarshal([]byte(readTest(t, backupPath)), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Servers["atlassian"]) != 2 {
		t.Fatalf("second clash: backup has %d definitions, want 2", len(b.Servers["atlassian"]))
	}

	// The same definition again, written with other whitespace, must not be appended.
	syncClaude(t, dir, pool(nil), proxyOK)
	writeTest(t, cfg, `{"mcpServers":{"atlassian":  {"type":"http","url":"https://other.example/mcp"}}}`)
	syncClaude(t, dir, pool(map[string]mcp.Server{"atlassian": atlassian}), proxyOK)
	b.Servers = nil
	if err := json.Unmarshal([]byte(readTest(t, backupPath)), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Servers["atlassian"]) != 2 {
		t.Fatalf("byte-equal definition appended: backup has %d, want 2", len(b.Servers["atlassian"]))
	}
}

func TestSyncLeavesOtherHandMadeServersAlone(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".claude.json")
	writeTest(t, cfg, `{"mcpServers":{"linear":{"type":"stdio","command":"linear-mcp"}}}`)
	r := syncClaude(t, dir, pool(map[string]mcp.Server{"docs": docs}), proxyOK)
	wantLines(t, r, "+ docs")
	if got := entry(t, cfg, "linear")["command"]; got != "linear-mcp" {
		t.Fatalf("linear changed: %v", got)
	}
	r = syncClaude(t, dir, pool(nil), proxyOK)
	wantLines(t, r, "- docs")
	if got := serverNames(t, cfg); !slices.Equal(got, []string{"linear"}) {
		t.Fatalf("servers = %v, want only linear", got)
	}
}

func TestSyncRemovalDropsManagedServerAndKeepsBackup(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".claude.json")
	backupPath := filepath.Join(dir, ".ap-mcp-backup.json")
	writeTest(t, cfg, `{"mcpServers":{"linear":{"type":"stdio","command":"linear-mcp"}}}`)
	writeTest(t, backupPath, `{"version":1,"servers":{"sentry":[{"command":"old"}]}}`)
	backupBefore := readTest(t, backupPath)

	syncClaude(t, dir, pool(map[string]mcp.Server{"atlassian": atlassian, "docs": docs}), proxyOK)
	r := syncClaude(t, dir, pool(map[string]mcp.Server{"docs": docs}), proxyOK)
	wantLines(t, r, "- atlassian")
	if got := serverNames(t, cfg); !slices.Equal(got, []string{"docs", "linear"}) {
		t.Fatalf("servers = %v", got)
	}
	if got := readTest(t, filepath.Join(dir, ".ap-mcp.json")); got != `["docs"]` {
		t.Fatalf("ledger = %s", got)
	}
	if got := readTest(t, backupPath); got != backupBefore {
		t.Fatalf("backup changed:\n%s", got)
	}
}

func TestSyncProxyUnavailableKeepsManagedOAuthEntry(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".claude.json")
	syncClaude(t, dir, pool(map[string]mcp.Server{"atlassian": atlassian, "docs": docs}), proxyOK)
	before := entry(t, cfg, "atlassian")

	fresh := mcp.Server{Command: "fresh-mcp"}
	r := syncClaude(t, dir, pool(map[string]mcp.Server{"atlassian": atlassian, "docs": docs, "fresh": fresh}), proxyDown)
	wantLines(t, r, "! atlassian mcporter unavailable: "+proxyDown.Unavailable, "+ fresh")
	if got := entry(t, cfg, "atlassian"); !equalDecoded(got, before) {
		t.Fatalf("atlassian entry changed: %v", got)
	}
	if got := readTest(t, filepath.Join(dir, ".ap-mcp.json")); got != `["atlassian","docs","fresh"]` {
		t.Fatalf("ledger = %s; atlassian must stay Managed", got)
	}

	// A Profile that never had atlassian gets no entry for it while mcporter is unavailable.
	other := t.TempDir()
	r = syncClaude(t, other, pool(map[string]mcp.Server{"atlassian": atlassian, "docs": docs}), proxyDown)
	wantLines(t, r, "! atlassian mcporter unavailable: "+proxyDown.Unavailable, "+ docs")
	if got := serverNames(t, filepath.Join(other, ".claude.json")); !slices.Equal(got, []string{"docs"}) {
		t.Fatalf("servers = %v, want only docs", got)
	}
}

func equalDecoded(a, b map[string]any) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return bytes.Equal(ja, jb)
}

func TestSyncSkipReasonPassesThrough(t *testing.T) {
	sentry := mcp.Server{Type: "sse", URL: "https://sentry.example/sse"}
	p := pool(map[string]mcp.Server{"atlassian": atlassian, "sentry": sentry})

	// The Claude Adapter can run SSE, so sentry is added there.
	claudeDir := t.TempDir()
	wantLines(t, syncClaude(t, claudeDir, p, proxyOK), "+ atlassian", "+ sentry")

	// The Pi-like Agent skips it, and the reason passes through; atlassian still syncs.
	piDir := t.TempDir()
	r, err := mcp.Sync(jsonSetter{}, piDir, p, proxyOK)
	if err != nil {
		t.Fatal(err)
	}
	wantLines(t, r, "+ atlassian", "! sentry SSE not supported by pi")
	var top struct {
		McpServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal([]byte(readTest(t, filepath.Join(piDir, "mcp.json"))), &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top.McpServers["sentry"]; ok {
		t.Fatal("skipped sentry was written")
	}
	if _, ok := top.McpServers["atlassian"]; !ok {
		t.Fatal("atlassian was not written to the Pi-like Agent")
	}
}

func TestSyncMalformedLedgerIsError(t *testing.T) {
	dir := t.TempDir()
	writeTest(t, filepath.Join(dir, ".ap-mcp.json"), `[oops`)
	if _, err := mcp.Sync(claude.Adapter{}, dir, pool(map[string]mcp.Server{"docs": docs}), proxyOK); err == nil {
		t.Fatal("want error for a malformed ledger")
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude.json")); !os.IsNotExist(err) {
		t.Fatalf("Agent config written despite the error: %v", err)
	}
}

func TestSyncLedgerMustBeArrayOfNames(t *testing.T) {
	for name, data := range map[string]string{
		"null":          `null`,
		"object":        `{"docs":true}`,
		"non-string":    `[1]`,
		"string":        `"docs"`,
		"empty content": ``,
		"null element":  `[null,"docs"]`,
		"empty name":    `[""]`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			cfg := filepath.Join(dir, ".claude.json")
			writeTest(t, cfg, `{"mcpServers":{"linear":{"type":"stdio","command":"linear-mcp"}}}`)
			ledger := filepath.Join(dir, ".ap-mcp.json")
			writeTest(t, ledger, data)
			cfgBefore := readTest(t, cfg)
			if _, err := mcp.Sync(claude.Adapter{}, dir, pool(map[string]mcp.Server{"docs": docs}), proxyOK); err == nil {
				t.Fatal("want error for a ledger that is not an array of names")
			}
			if got := readTest(t, cfg); got != cfgBefore {
				t.Fatalf("Agent config changed despite the error:\n%s", got)
			}
			if got := readTest(t, ledger); got != data {
				t.Fatalf("ledger was reset to %q", got)
			}
			if _, err := os.Stat(filepath.Join(dir, ".ap-mcp-backup.json")); !os.IsNotExist(err) {
				t.Fatalf("backup written despite the error: %v", err)
			}
		})
	}
}

func TestSyncIdenticalHandMadeServerIsBackedUpAndReported(t *testing.T) {
	dir := t.TempDir()
	backupPath := filepath.Join(dir, ".ap-mcp-backup.json")
	p := pool(map[string]mcp.Server{"docs": docs})

	// Make the Profile's docs entry identical to the pool's, by hand, with no ledger.
	syncClaude(t, dir, p, proxyOK)
	if err := os.Remove(filepath.Join(dir, ".ap-mcp.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("backup exists before the clash: %v", err)
	}

	r := syncClaude(t, dir, p, proxyOK)
	wantLines(t, r, "! docs replaced by pool; backup saved")
	var b struct {
		Servers map[string][]json.RawMessage `json:"servers"`
	}
	if err := json.Unmarshal([]byte(readTest(t, backupPath)), &b); err != nil {
		t.Fatal(err)
	}
	if len(b.Servers["docs"]) != 1 {
		t.Fatalf("backup has %d definitions for docs, want 1", len(b.Servers["docs"]))
	}
	if got := readTest(t, filepath.Join(dir, ".ap-mcp.json")); got != `["docs"]` {
		t.Fatalf("ledger = %s", got)
	}

	// A second run is a plain no-change run: the server is Managed now and no backup is added.
	before := readTest(t, backupPath)
	wantLines(t, syncClaude(t, dir, p, proxyOK))
	if got := readTest(t, backupPath); got != before {
		t.Fatalf("backup changed on a second run:\n%s", got)
	}
}

func TestSyncMalformedBackupIsError(t *testing.T) {
	for name, data := range map[string]string{
		"malformed":       `{nope`,
		"unknown version": `{"version":2,"servers":{}}`,
		"missing version": `{"servers":{}}`,
		"missing servers": `{"version":1}`,
		"null servers":    `{"version":1,"servers":null}`,
		"array servers":   `{"version":1,"servers":[]}`,
		"null list":       `{"version":1,"servers":{"docs":null}}`,
		"null document":   `null`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeTest(t, filepath.Join(dir, ".ap-mcp-backup.json"), data)
			if _, err := mcp.Sync(claude.Adapter{}, dir, pool(map[string]mcp.Server{"docs": docs}), proxyOK); err == nil {
				t.Fatal("want error")
			}
			if _, err := os.Stat(filepath.Join(dir, ".claude.json")); !os.IsNotExist(err) {
				t.Fatalf("Agent config written despite the error: %v", err)
			}
			if got := readTest(t, filepath.Join(dir, ".ap-mcp-backup.json")); got != data {
				t.Fatalf("backup was reset to %q", got)
			}
		})
	}
}

func TestSyncRefusesSymlinkedAgentConfig(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "real.json")
	writeTest(t, target, `{}`)
	if err := os.Symlink(target, filepath.Join(dir, ".claude.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := mcp.Sync(claude.Adapter{}, dir, pool(map[string]mcp.Server{"docs": docs}), proxyOK); err == nil {
		t.Fatal("want error for a symlinked Agent config")
	}
	if got := readTest(t, target); got != `{}` {
		t.Fatalf("symlink target changed: %s", got)
	}
}

func TestSyncNoChangeWritesNothing(t *testing.T) {
	dir := t.TempDir()
	p := pool(map[string]mcp.Server{"atlassian": atlassian, "docs": docs})
	syncClaude(t, dir, p, proxyOK)

	past := time.Date(2020, 1, 2, 3, 4, 5, 0, time.UTC)
	files := []string{filepath.Join(dir, ".claude.json"), filepath.Join(dir, ".ap-mcp.json")}
	for _, f := range files {
		if err := os.Chtimes(f, past, past); err != nil {
			t.Fatal(err)
		}
	}
	r := syncClaude(t, dir, p, proxyOK)
	wantLines(t, r)
	if r.Changed() {
		t.Fatal("Changed = true on a no-change run")
	}
	for _, f := range files {
		st, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if !st.ModTime().Equal(past) {
			t.Fatalf("%s was rewritten (mtime %v)", filepath.Base(f), st.ModTime())
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".ap-mcp-backup.json")); !os.IsNotExist(err) {
		t.Fatalf("backup created on a no-change run: %v", err)
	}
}

func TestSyncEmptyProfileWritesNoLedgerWhenPoolIsEmpty(t *testing.T) {
	dir := t.TempDir()
	r := syncClaude(t, dir, pool(nil), proxyOK)
	wantLines(t, r)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("files created for an empty pool: %v", entries)
	}
}

func TestSyncTranslateErrorNamesServer(t *testing.T) {
	_, err := mcp.Sync(jsonSetter{}, t.TempDir(), pool(map[string]mcp.Server{"boom": docs}), proxyOK)
	if err == nil || !strings.Contains(err.Error(), `"boom"`) {
		t.Fatalf("err = %v, want one naming \"boom\"", err)
	}
}
