package mcp

import "encoding/json"

// Setter is implemented by Adapters that can write MCP servers into an
// Agent's config. internal/mcp never imports internal/agent; the Adapters
// satisfy this interface structurally.
type Setter interface {
	// MCPConfigFile returns the path of the Agent's MCP config inside dir.
	MCPConfigFile(dir string) string
	// Translate returns the Agent's entry for a Pool server, or a skip reason
	// when the Agent cannot run it (an empty skip means the entry is usable).
	Translate(name string, s Server, proxy Proxy) (entry json.RawMessage, skip string, err error)
	// SetServers writes next, drops prev Managed servers not in next, and
	// keeps everything else. It returns current byte-identical when nothing changed.
	SetServers(current []byte, prev []string, next map[string]json.RawMessage) ([]byte, error)
}

// OAuthEntry returns the stdio entry that runs an OAuth Pool server through
// mcporter, using the Shared login. Every Agent writes this same entry.
func OAuthEntry(name string, proxy Proxy) map[string]any {
	return map[string]any{
		"command": "mcporter",
		"args":    []string{"serve", "--stdio", "--servers", name},
		"env": map[string]string{
			"MCPORTER_CONFIG":           proxy.ConfigPath,
			"MCPORTER_OAUTH_NO_BROWSER": "1",
		},
	}
}
