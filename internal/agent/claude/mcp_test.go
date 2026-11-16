package claude_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/agent/claude"
	"github.com/thlinhit/agent-profiles/internal/mcp"
)

const mcporterPath = "/data/agent-profiles/mcporter.json"

func TestMCPTranslate(t *testing.T) {
	ok := mcp.Proxy{ConfigPath: mcporterPath}
	for _, tc := range []struct {
		name   string
		server mcp.Server
		proxy  mcp.Proxy
		want   string
		skip   string
	}{
		{
			name:   "direct stdio",
			server: mcp.Server{Command: "npx", Args: []string{"-y", "tool"}, Env: map[string]string{"K": "${TOKEN}"}},
			want:   `{"args":["-y","tool"],"command":"npx","env":{"K":"${TOKEN}"},"type":"stdio"}`,
		},
		{
			name:   "direct http keeps ${VAR} header literally",
			server: mcp.Server{Type: "http", URL: "https://x.example/mcp", Headers: map[string]string{"Authorization": "Bearer ${API_TOKEN}"}},
			want:   `{"headers":{"Authorization":"Bearer ${API_TOKEN}"},"type":"http","url":"https://x.example/mcp"}`,
		},
		{
			name:   "direct http defaults type and omits empty fields",
			server: mcp.Server{URL: "https://x.example/mcp"},
			want:   `{"type":"http","url":"https://x.example/mcp"}`,
		},
		{
			name:   "direct sse keeps its type",
			server: mcp.Server{Type: "sse", URL: "https://x.example/sse"},
			want:   `{"type":"sse","url":"https://x.example/sse"}`,
		},
		{
			name:   "oauth with proxy available",
			server: mcp.Server{Type: "http", URL: "https://mcp.example/v1", Auth: "oauth"},
			proxy:  ok,
			want:   `{"args":["serve","--stdio","--servers","atlassian"],"command":"mcporter","env":{"MCPORTER_CONFIG":"/data/agent-profiles/mcporter.json","MCPORTER_OAUTH_NO_BROWSER":"1"},"type":"stdio"}`,
		},
		{
			name:   "oauth skipped when proxy unavailable",
			server: mcp.Server{Type: "http", URL: "https://mcp.example/v1", Auth: "oauth"},
			proxy:  mcp.Proxy{ConfigPath: mcporterPath, Unavailable: "mcporter not found"},
			skip:   "mcporter unavailable: mcporter not found",
		},
		{
			name:   "agent-only extra fields are kept",
			server: mcp.Server{Command: "srv", Extra: map[string]json.RawMessage{"timeout": json.RawMessage(`5000`), "exposure": json.RawMessage(`{"tools":["a"]}`)}},
			want:   `{"command":"srv","exposure":{"tools":["a"]},"timeout":5000,"type":"stdio"}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry, skip, err := (claude.Adapter{}).Translate("atlassian", tc.server, tc.proxy)
			if err != nil {
				t.Fatal(err)
			}
			if skip != tc.skip {
				t.Fatalf("skip = %q; want %q", skip, tc.skip)
			}
			if tc.skip != "" {
				if entry != nil {
					t.Fatalf("skipped entry = %s; want nil", entry)
				}
				return
			}
			if string(entry) != tc.want {
				t.Fatalf("entry = %s\nwant    %s", entry, tc.want)
			}
		})
	}
}

func TestMCPConfigFile(t *testing.T) {
	if got := (claude.Adapter{}).MCPConfigFile("/p/claude/work"); got != "/p/claude/work/.claude.json" {
		t.Fatalf("MCPConfigFile = %q", got)
	}
}

func TestMCPSetServers(t *testing.T) {
	const current = `{
  "numStartups": 4,
  "mcpServers": {
    "linear": {"type": "http", "url": "https://linear.example/mcp"},
    "atlassian": {"command": "old"},
    "gone": {"command": "bye"}
  },
  "projects": {"/x": {"allowed": true}}
}
`
	next := map[string]json.RawMessage{
		"atlassian": json.RawMessage(`{"command":"mcporter","type":"stdio"}`),
		"sentry":    json.RawMessage(`{"type":"http","url":"https://s.example"}`),
	}
	prev := []string{"atlassian", "gone"}

	t.Run("adds updates and removes managed servers", func(t *testing.T) {
		out, err := (claude.Adapter{}).SetServers([]byte(current), prev, next)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		wantServers := map[string]any{
			"linear":    map[string]any{"type": "http", "url": "https://linear.example/mcp"},
			"atlassian": map[string]any{"command": "mcporter", "type": "stdio"},
			"sentry":    map[string]any{"type": "http", "url": "https://s.example"},
		}
		if !reflect.DeepEqual(got["mcpServers"], wantServers) {
			t.Fatalf("mcpServers = %v", got["mcpServers"])
		}
		if !reflect.DeepEqual(got["numStartups"], float64(4)) || !reflect.DeepEqual(got["projects"], map[string]any{"/x": map[string]any{"allowed": true}}) {
			t.Fatalf("other top-level keys changed: %s", out)
		}
		if !strings.HasSuffix(string(out), "}\n") || !strings.Contains(string(out), "\n  \"mcpServers\"") {
			t.Fatalf("want 2-space indent and trailing newline, got %q", out)
		}
	})

	t.Run("unchanged result is byte-identical", func(t *testing.T) {
		once, err := (claude.Adapter{}).SetServers([]byte(current), prev, next)
		if err != nil {
			t.Fatal(err)
		}
		again, err := (claude.Adapter{}).SetServers(once, prev, next)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(once) {
			t.Fatalf("second run changed bytes:\n%s\n---\n%s", once, again)
		}
		// Reformatted but semantically equal input must come back untouched too.
		squashed := `{"mcpServers":{"linear":{"url":"https://linear.example/mcp","type":"http"}},"numStartups":4}`
		out, err := (claude.Adapter{}).SetServers([]byte(squashed), nil, map[string]json.RawMessage{"linear": json.RawMessage(`{"type":"http","url":"https://linear.example/mcp"}`)})
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != squashed {
			t.Fatalf("unchanged input was rewritten: %s", out)
		}
	})

	t.Run("large integers differing past float64 precision are updated", func(t *testing.T) {
		cur := `{"mcpServers":{"srv":{"timeout":9007199254740992}}}`
		out, err := (claude.Adapter{}).SetServers([]byte(cur), nil, map[string]json.RawMessage{"srv": json.RawMessage(`{"timeout":9007199254740993}`)})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "9007199254740993") {
			t.Fatalf("update was dropped: %s", out)
		}
	})

	t.Run("hand-made server with 1e400 is left alone without error", func(t *testing.T) {
		cur := `{"mcpServers":{"hand":{"command":"x","timeout":1e400}}}`
		out, err := (claude.Adapter{}).SetServers([]byte(cur), nil, map[string]json.RawMessage{"atlassian": json.RawMessage(`{"command":"mcporter"}`)})
		if err != nil {
			t.Fatalf("SetServers error: %v", err)
		}
		var got struct {
			MCPServers map[string]json.RawMessage `json:"mcpServers"`
		}
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, got.MCPServers["hand"]); err != nil {
			t.Fatal(err)
		}
		if compact.String() != `{"command":"x","timeout":1e400}` {
			t.Fatalf("hand-made server changed: %s", compact.String())
		}
		if _, ok := got.MCPServers["atlassian"]; !ok {
			t.Fatalf("managed server not added: %s", out)
		}
	})

	t.Run("nil current creates mcpServers", func(t *testing.T) {
		out, err := (claude.Adapter{}).SetServers(nil, nil, map[string]json.RawMessage{"atlassian": json.RawMessage(`{"type":"stdio","command":"mcporter"}`)})
		if err != nil {
			t.Fatal(err)
		}
		// Entries are written as the raw bytes given, so their key order is kept.
		want := "{\n  \"mcpServers\": {\n    \"atlassian\": {\n      \"type\": \"stdio\",\n      \"command\": \"mcporter\"\n    }\n  }\n}\n"
		if string(out) != want {
			t.Fatalf("SetServers(nil) = %q; want %q", out, want)
		}
	})

	t.Run("empty current with nothing to write is unchanged", func(t *testing.T) {
		out, err := (claude.Adapter{}).SetServers(nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		if out != nil {
			t.Fatalf("SetServers(nil, nil, nil) = %q; want nil", out)
		}
	})

	t.Run("malformed current is an error", func(t *testing.T) {
		if _, err := (claude.Adapter{}).SetServers([]byte(`[1]`), nil, nil); err == nil {
			t.Fatal("want error for non-object current")
		}
	})
}
