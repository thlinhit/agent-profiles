package pi_test

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/agent/pi"
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
			want:   `{"args":["-y","tool"],"command":"npx","env":{"K":"${TOKEN}"}}`,
		},
		{
			name:   "direct stdio omits empty args and env",
			server: mcp.Server{Command: "npx"},
			want:   `{"command":"npx"}`,
		},
		{
			name:   "direct http keeps ${VAR} header literally and omits type",
			server: mcp.Server{Type: "http", URL: "https://x.example/mcp", Headers: map[string]string{"Authorization": "Bearer ${API_TOKEN}"}},
			want:   `{"headers":{"Authorization":"Bearer ${API_TOKEN}"},"url":"https://x.example/mcp"}`,
		},
		{
			name:   "direct http omits empty headers",
			server: mcp.Server{URL: "https://x.example/mcp"},
			want:   `{"url":"https://x.example/mcp"}`,
		},
		{
			name:   "oauth with proxy available",
			server: mcp.Server{Type: "http", URL: "https://mcp.example/v1", Auth: "oauth"},
			proxy:  ok,
			want:   `{"args":["serve","--stdio","--servers","atlassian"],"command":"mcporter","env":{"MCPORTER_CONFIG":"/data/agent-profiles/mcporter.json","MCPORTER_OAUTH_NO_BROWSER":"1"}}`,
		},
		{
			name:   "oauth skipped when proxy unavailable",
			server: mcp.Server{Type: "http", URL: "https://mcp.example/v1", Auth: "oauth"},
			proxy:  mcp.Proxy{ConfigPath: mcporterPath, Unavailable: "mcporter not found"},
			skip:   "mcporter unavailable: mcporter not found",
		},
		{
			name:   "oauth with ${VAR:-default} URL still gives the mcporter entry",
			server: mcp.Server{Type: "http", URL: "https://${HOST:-mcp.example}/v1", Auth: "oauth", Headers: map[string]string{"X": "${T:-y}"}},
			proxy:  ok,
			want:   `{"args":["serve","--stdio","--servers","atlassian"],"command":"mcporter","env":{"MCPORTER_CONFIG":"/data/agent-profiles/mcporter.json","MCPORTER_OAUTH_NO_BROWSER":"1"}}`,
		},
		{
			name:   "oauth unavailable with ${VAR:-default} URL gives the proxy reason",
			server: mcp.Server{URL: "https://${HOST:-mcp.example}/v1", Auth: "oauth"},
			proxy:  mcp.Proxy{ConfigPath: mcporterPath, Unavailable: "mcporter not found"},
			skip:   "mcporter unavailable: mcporter not found",
		},
		{
			name:   "sse is skipped",
			server: mcp.Server{Type: "sse", URL: "https://s.example/sse"},
			skip:   "SSE not supported by pi",
		},
		{
			name:   "env with ${VAR:-default} is skipped",
			server: mcp.Server{Command: "srv", Env: map[string]string{"K": "${TOKEN:-none}"}},
			skip:   "${VAR:-default} not supported by pi",
		},
		{
			name:   "header with ${VAR:-default} is skipped",
			server: mcp.Server{URL: "https://x.example/mcp", Headers: map[string]string{"Authorization": "Bearer ${API_TOKEN:-x}"}},
			skip:   "${VAR:-default} not supported by pi",
		},
		{
			name:   "url with ${VAR:-default} is skipped",
			server: mcp.Server{URL: "https://${HOST:-x.example}/mcp"},
			skip:   "${VAR:-default} not supported by pi",
		},
		{
			name:   "agent-only extra fields are kept",
			server: mcp.Server{Command: "srv", Extra: map[string]json.RawMessage{"cwd": json.RawMessage(`"/work"`), "exposure": json.RawMessage(`"deferred"`), "toolExposure": json.RawMessage(`{"a":"direct"}`)}},
			want:   `{"command":"srv","cwd":"/work","exposure":"deferred","toolExposure":{"a":"direct"}}`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry, skip, err := (pi.Adapter{}).Translate("atlassian", tc.server, tc.proxy)
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
	if got := (pi.Adapter{}).MCPConfigFile("/p/pi/work"); got != "/p/pi/work/mcp.json" {
		t.Fatalf("MCPConfigFile = %q", got)
	}
}

func TestMCPSetServers(t *testing.T) {
	const current = `{
  "theme": "dark",
  "mcpServers": {
    "linear": {"url": "https://linear.example/mcp"},
    "atlassian": {"command": "old"},
    "gone": {"command": "bye"},
    "hand": {"command": "mine", "env": {"K": "v"}}
  },
  "defaultModel": "x"
}
`
	next := map[string]json.RawMessage{
		"atlassian": json.RawMessage(`{"command":"mcporter","args":["serve"]}`),
		"sentry":    json.RawMessage(`{"url":"https://s.example"}`),
	}
	prev := []string{"atlassian", "gone"}

	t.Run("adds updates and removes managed servers, leaves hand-made ones", func(t *testing.T) {
		out, err := (pi.Adapter{}).SetServers([]byte(current), prev, next)
		if err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		if err := json.Unmarshal(out, &got); err != nil {
			t.Fatal(err)
		}
		wantServers := map[string]any{
			"linear":    map[string]any{"url": "https://linear.example/mcp"},
			"hand":      map[string]any{"command": "mine", "env": map[string]any{"K": "v"}},
			"atlassian": map[string]any{"command": "mcporter", "args": []any{"serve"}},
			"sentry":    map[string]any{"url": "https://s.example"},
		}
		if !reflect.DeepEqual(got["mcpServers"], wantServers) {
			t.Fatalf("mcpServers = %v", got["mcpServers"])
		}
		if got["theme"] != "dark" || got["defaultModel"] != "x" {
			t.Fatalf("other top-level keys changed: %s", out)
		}
		if !strings.HasSuffix(string(out), "}\n") || !strings.Contains(string(out), "\n  \"mcpServers\"") {
			t.Fatalf("want 2-space indent and trailing newline, got %q", out)
		}
	})

	t.Run("unchanged result is byte-identical", func(t *testing.T) {
		once, err := (pi.Adapter{}).SetServers([]byte(current), prev, next)
		if err != nil {
			t.Fatal(err)
		}
		again, err := (pi.Adapter{}).SetServers(once, prev, next)
		if err != nil {
			t.Fatal(err)
		}
		if string(again) != string(once) {
			t.Fatalf("second run changed bytes:\n%s\n---\n%s", once, again)
		}
		squashed := `{"mcpServers":{"linear":{"url":"https://linear.example/mcp","headers":{}}},"theme":"dark"}`
		out, err := (pi.Adapter{}).SetServers([]byte(squashed), nil, map[string]json.RawMessage{"linear": json.RawMessage(`{"headers":{},"url":"https://linear.example/mcp"}`)})
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != squashed {
			t.Fatalf("unchanged input was rewritten: %s", out)
		}
	})

	t.Run("large integers differing past float64 precision are updated", func(t *testing.T) {
		cur := `{"mcpServers":{"srv":{"timeout":9007199254740992}}}`
		out, err := (pi.Adapter{}).SetServers([]byte(cur), nil, map[string]json.RawMessage{"srv": json.RawMessage(`{"timeout":9007199254740993}`)})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(out), "9007199254740993") {
			t.Fatalf("update was dropped: %s", out)
		}
	})

	t.Run("hand-made server with 1e400 is left alone without error", func(t *testing.T) {
		cur := `{"mcpServers":{"hand":{"command":"x","timeout":1e400}}}`
		out, err := (pi.Adapter{}).SetServers([]byte(cur), nil, map[string]json.RawMessage{"atlassian": json.RawMessage(`{"command":"mcporter"}`)})
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
	})

	t.Run("nil current creates mcpServers", func(t *testing.T) {
		out, err := (pi.Adapter{}).SetServers(nil, nil, map[string]json.RawMessage{"atlassian": json.RawMessage(`{"command":"mcporter"}`)})
		if err != nil {
			t.Fatal(err)
		}
		want := "{\n  \"mcpServers\": {\n    \"atlassian\": {\n      \"command\": \"mcporter\"\n    }\n  }\n}\n"
		if string(out) != want {
			t.Fatalf("SetServers(nil) = %q; want %q", out, want)
		}
	})

	t.Run("malformed current is an error", func(t *testing.T) {
		if _, err := (pi.Adapter{}).SetServers([]byte(`[1]`), nil, nil); err == nil {
			t.Fatal("want error for non-object current")
		}
	})
}
