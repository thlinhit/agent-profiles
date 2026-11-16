package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func decodeAny(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	return v
}

const roundTripInput = `{
  "mcpServers": {
    "jira": {
      "type": "http",
      "url": "https://jira.example/mcp",
      "auth": "oauth",
      "exposure": {"tools": ["a", "b"], "nested": {"deep": true}}
    },
    "local": {
      "command": "node",
      "args": ["s.js"],
      "env": {"A": "${A}"},
      "secretCommand": "op read x",
      "extraList": [1, 2, {"k": "v"}]
    }
  },
  "futureTopLevel": {"x": 1}
}`

func TestPoolRoundTripKeepsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	out := filepath.Join(dir, "out.json")
	writeFile(t, in, roundTripInput)

	p, exists, err := Load(in)
	if err != nil || !exists {
		t.Fatalf("Load: exists=%v err=%v", exists, err)
	}
	if err := Save(out, p); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decodeAny(t, []byte(roundTripInput)), decodeAny(t, got)) {
		t.Fatalf("round trip lost or changed fields:\n%s", got)
	}

	p2, _, err := Load(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := Save(out, p2); err != nil {
		t.Fatal(err)
	}
	again, _ := os.ReadFile(out)
	if string(again) != string(got) {
		t.Fatalf("second save differs from first")
	}
}

func TestPoolRoundTripKeepsDifferentlyCasedKeys(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	out := filepath.Join(dir, "out.json")
	input := `{"mcpServers":{"s":{"command":"safe","Command":"other","Command2":{}}}}`
	writeFile(t, in, `{"mcpServers":{"s":{"command":"safe","Command":"other","Command2":{}}}}`)
	p, _, err := Load(in)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if p.Servers["s"].Command != "safe" {
		t.Fatalf("Command = %q, want safe", p.Servers["s"].Command)
	}
	if err := Save(out, p); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if !reflect.DeepEqual(decodeAny(t, []byte(input)), decodeAny(t, got)) {
		t.Fatalf("differently cased keys changed:\n%s", got)
	}
}

// The brief's exact case: a lowercase command plus an unknown "Command" with an object value.
func TestPoolRoundTripCommandCaseCollision(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	out := filepath.Join(dir, "out.json")
	input := `{"mcpServers":{"s":{"command":"safe","Command":"other"}}}`
	writeFile(t, in, input)
	p, _, err := Load(in)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if err := Save(out, p); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if !reflect.DeepEqual(decodeAny(t, []byte(input)), decodeAny(t, got)) {
		t.Fatalf("round trip changed command:\n%s", got)
	}
	if !strings.Contains(string(got), `"command": "safe"`) {
		t.Fatalf("lowercase command was overwritten:\n%s", got)
	}
}

func TestPoolRoundTripUnknownCapitalisedObject(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "in.json")
	input := `{"mcpServers":{"s":{"command":"safe","Command":{}}}}`
	writeFile(t, in, input)
	p, exists, err := Load(in)
	if err != nil || !exists {
		t.Fatalf("Load with unknown \"Command\":{} must succeed: exists=%v err=%v", exists, err)
	}
	if p.Servers["s"].Command != "safe" {
		t.Fatalf("Command = %q, want safe", p.Servers["s"].Command)
	}
	out := filepath.Join(dir, "out.json")
	if err := Save(out, p); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(out)
	if !reflect.DeepEqual(decodeAny(t, []byte(input)), decodeAny(t, got)) {
		t.Fatalf("round trip changed unknown Command object:\n%s", got)
	}
}

func TestPoolLoadMissingFileNotCreated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	p, exists, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if exists {
		t.Fatal("exists = true for missing file")
	}
	if p.Servers == nil || len(p.Servers) != 0 {
		t.Fatalf("want empty non-nil Servers, got %#v", p.Servers)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("Load created the file: %v", err)
	}
}

func TestPoolLoadRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	writeFile(t, target, `{"mcpServers":{}}`)
	link := filepath.Join(dir, "mcp.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(link); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("want symlink refusal, got %v", err)
	}
}

func TestPoolLoadRefusesNonRegularFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "non-regular") {
		t.Fatalf("want non-regular refusal, got %v", err)
	}
}

func TestPoolLoadRefusesTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	writeFile(t, path, strings.Repeat(" ", maxPoolSize+1))
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "larger than") {
		t.Fatalf("want size refusal, got %v", err)
	}
}

func TestPoolLoadRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	writeFile(t, path, `{"mcpServers": {`)
	if _, _, err := Load(path); err == nil || !strings.Contains(err.Error(), "malformed JSON") {
		t.Fatalf("want malformed JSON error, got %v", err)
	}
}

func TestPoolLoadRejectsBadMCPServers(t *testing.T) {
	cases := map[string]string{
		"null":           `{"mcpServers": null}`,
		"array":          `{"mcpServers": []}`,
		"string":         `{"mcpServers": "x"}`,
		"null server":    `{"mcpServers": {"a": null}}`,
		"scalar server":  `{"mcpServers": {"a": 5}}`,
		"top-level null": `null`,
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mcp.json")
			writeFile(t, path, content)
			if _, _, err := Load(path); err == nil {
				t.Fatalf("want error for %s", content)
			}
		})
	}
}

func TestPoolSaveWritesSortedJSONWithNewline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "mcp.json")
	p := Pool{Servers: map[string]Server{
		"zeta":  {URL: "https://z.example", Type: "http"},
		"alpha": {Command: "a", Args: []string{"x"}},
	}}
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(string(got), "}\n") {
		t.Fatalf("missing trailing newline: %q", got)
	}
	if strings.Index(string(got), `"alpha"`) > strings.Index(string(got), `"zeta"`) {
		t.Fatalf("keys not sorted:\n%s", got)
	}
}

func TestPoolSaveNoChangeKeepsMtime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mcp.json")
	p := Pool{Servers: map[string]Server{"a": {Command: "x"}}}
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, past, past); err != nil {
		t.Fatal(err)
	}
	if err := Save(path, p); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !st.ModTime().Equal(past) {
		t.Fatalf("unchanged save rewrote the file: mtime %v, want %v", st.ModTime(), past)
	}
}

func TestPoolSaveMode0600AndDir0700(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new")
	path := filepath.Join(dir, "mcp.json")
	if err := Save(path, Pool{Servers: map[string]Server{"a": {Command: "x"}}}); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := st.Mode().Perm(); perm != 0o600 {
		t.Fatalf("file mode = %o, want 600", perm)
	}
	dst, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := dst.Mode().Perm(); perm != 0o700 {
		t.Fatalf("dir mode = %o, want 700", perm)
	}
}

func TestPoolSaveRefusesSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	writeFile(t, target, "keep")
	link := filepath.Join(dir, "mcp.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if err := Save(link, Pool{Servers: map[string]Server{"a": {Command: "x"}}}); err == nil {
		t.Fatal("want symlink refusal")
	}
	if got, _ := os.ReadFile(target); string(got) != "keep" {
		t.Fatalf("symlink target was clobbered: %q", got)
	}
}

func TestValidate(t *testing.T) {
	stdio := Server{Command: "node"}
	http := Server{URL: "https://x.example/mcp"}
	cases := []struct {
		name    string
		server  string
		s       Server
		wantErr string
	}{
		{"valid stdio", "jira-1", stdio, ""},
		{"valid http", "a_b-c", http, ""},
		{"name max length", strings.Repeat("a", 64), stdio, ""},
		{"name too long", strings.Repeat("a", 65), stdio, "invalid MCP server name"},
		{"name uppercase", "Jira", stdio, "invalid MCP server name"},
		{"name leading dash", "-jira", stdio, "invalid MCP server name"},
		{"name leading underscore", "_jira", stdio, "invalid MCP server name"},
		{"name double underscore", "a__b", stdio, "invalid MCP server name"},
		{"name space", "a b", stdio, "invalid MCP server name"},
		{"neither command nor url", "a", Server{}, "exactly one of command or url"},
		{"both command and url", "a", Server{Command: "x", URL: "https://x"}, "exactly one of command or url"},
		{"type stdio", "a", Server{Type: "stdio", Command: "x"}, ""},
		{"type http", "a", Server{Type: "http", URL: "https://x"}, ""},
		{"type sse", "a", Server{Type: "sse", URL: "https://x"}, ""},
		{"type unknown", "a", Server{Type: "ws", URL: "https://x"}, "type must be stdio, http or sse"},
		{"auth oauth with url", "a", Server{Auth: "oauth", URL: "https://x"}, ""},
		{"auth oauth without url", "a", Server{Auth: "oauth", Command: "x"}, "auth oauth requires url"},
		{"auth unknown", "a", Server{Auth: "basic", URL: "https://x"}, "auth must be empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := Validate(tc.server, tc.s)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
