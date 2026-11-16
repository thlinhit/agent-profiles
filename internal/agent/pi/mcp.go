package pi

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"

	"github.com/thlinhit/agent-profiles/internal/mcp"
)

// defaultSyntax matches ${VAR:-default}, which Pi does not expand.
var defaultSyntax = regexp.MustCompile(`\$\{[A-Za-z_][A-Za-z0-9_]*:-`)

// MCPConfigFile returns the file Pi reads MCP servers from.
func (Adapter) MCPConfigFile(dir string) string {
	return filepath.Join(dir, "mcp.json")
}

// Translate returns Pi's entry for a Pool server. OAuth servers run through
// mcporter, or are skipped when mcporter is unavailable. Pi rejects SSE and
// does not expand ${VAR:-default}, so those servers are skipped. Other
// servers are written directly. Agent-only fields in Extra are kept as they are.
func (Adapter) Translate(name string, s mcp.Server, proxy mcp.Proxy) (json.RawMessage, string, error) {
	entry := make(map[string]any, len(s.Extra)+4)
	for k, v := range s.Extra {
		entry[k] = v
	}
	switch {
	case s.Auth == "oauth":
		// The OAuth entry is only "mcporter serve --servers <name>": the URL,
		// headers and env of the Pool server never reach Pi. So ${VAR:-default}
		// in them does not matter here and the server is not skipped for it.
		if proxy.Unavailable != "" {
			return nil, "mcporter unavailable: " + proxy.Unavailable, nil
		}
		for k, v := range mcp.OAuthEntry(name, proxy) {
			entry[k] = v
		}
	case s.Type == "sse":
		return nil, "SSE not supported by pi", nil
	default:
		// Direct servers write their URL, headers and env to Pi, so all three
		// are checked before any entry is built.
		if reason := unsupportedDefault(s.URL, s.Headers, s.Env); reason != "" {
			return nil, reason, nil
		}
		if s.Command != "" {
			entry["command"] = s.Command
			if len(s.Args) > 0 {
				entry["args"] = s.Args
			}
			if len(s.Env) > 0 {
				entry["env"] = s.Env
			}
		} else {
			entry["url"] = s.URL
			if len(s.Headers) > 0 {
				entry["headers"] = s.Headers
			}
		}
	}
	out, err := json.Marshal(entry)
	if err != nil {
		return nil, "", fmt.Errorf("MCP server %q: %w", name, err)
	}
	return out, "", nil
}

// unsupportedDefault returns a skip reason when any value uses ${VAR:-default}.
func unsupportedDefault(values ...any) string {
	for _, v := range values {
		var hit bool
		switch v := v.(type) {
		case string:
			hit = defaultSyntax.MatchString(v)
		case map[string]string:
			for _, s := range v {
				hit = hit || defaultSyntax.MatchString(s)
			}
		}
		if hit {
			return "${VAR:-default} not supported by pi"
		}
	}
	return ""
}

// SetServers writes next into the mcpServers of current, drops prev names that
// are not in next, and leaves every other key and server untouched.
// When the resulting mcpServers equals the current one, current is returned unchanged.
func (Adapter) SetServers(current []byte, prev []string, next map[string]json.RawMessage) ([]byte, error) {
	return mcp.MergeServers(current, prev, next)
}
