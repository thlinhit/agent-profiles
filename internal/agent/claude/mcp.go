package claude

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/thlinhit/agent-profiles/internal/mcp"
)

// MCPConfigFile returns the file Claude Code reads MCP servers from.
func (Adapter) MCPConfigFile(dir string) string {
	return filepath.Join(dir, ".claude.json")
}

// Translate returns Claude's entry for a Pool server. OAuth servers run through
// mcporter, or are skipped when mcporter is unavailable. Other servers are
// written directly. Agent-only fields in Extra are kept as they are.
func (Adapter) Translate(name string, s mcp.Server, proxy mcp.Proxy) (json.RawMessage, string, error) {
	entry := make(map[string]any, len(s.Extra)+5)
	for k, v := range s.Extra {
		entry[k] = v
	}
	switch {
	case s.Auth == "oauth":
		if proxy.Unavailable != "" {
			return nil, "mcporter unavailable: " + proxy.Unavailable, nil
		}
		entry["type"] = "stdio"
		for k, v := range mcp.OAuthEntry(name, proxy) {
			entry[k] = v
		}
	case s.Command != "":
		entry["type"] = "stdio"
		entry["command"] = s.Command
		if len(s.Args) > 0 {
			entry["args"] = s.Args
		}
		if len(s.Env) > 0 {
			entry["env"] = s.Env
		}
	default:
		entry["type"] = "http"
		if s.Type != "" {
			entry["type"] = s.Type
		}
		entry["url"] = s.URL
		if len(s.Headers) > 0 {
			entry["headers"] = s.Headers
		}
	}
	out, err := json.Marshal(entry)
	if err != nil {
		return nil, "", fmt.Errorf("MCP server %q: %w", name, err)
	}
	return out, "", nil
}

// SetServers merges next into the mcpServers of current; see mcp.MergeServers.
func (Adapter) SetServers(current []byte, prev []string, next map[string]json.RawMessage) ([]byte, error) {
	return mcp.MergeServers(current, prev, next)
}
