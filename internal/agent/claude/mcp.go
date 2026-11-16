package claude

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"

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

// SetServers writes next into the top-level mcpServers of current, drops prev
// names that are not in next, and leaves every other key and server untouched.
// When the resulting mcpServers equals the current one, current is returned unchanged.
func (Adapter) SetServers(current []byte, prev []string, next map[string]json.RawMessage) ([]byte, error) {
	top := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(current)) > 0 {
		if err := json.Unmarshal(current, &top); err != nil {
			return nil, fmt.Errorf("parse MCP config: %w", err)
		}
		if top == nil {
			return nil, fmt.Errorf("parse MCP config: not a JSON object")
		}
	}
	servers := map[string]json.RawMessage{}
	if raw, ok := top["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil || servers == nil {
			return nil, fmt.Errorf("parse MCP config: mcpServers is not a JSON object")
		}
	}
	want := make(map[string]json.RawMessage, len(servers))
	for k, v := range servers {
		want[k] = v
	}
	for _, name := range prev {
		if _, keep := next[name]; !keep {
			delete(want, name)
		}
	}
	for name, entry := range next {
		want[name] = entry
	}
	same, err := sameServers(servers, want)
	if err != nil {
		return nil, err
	}
	if same {
		return current, nil
	}
	merged, err := json.Marshal(want)
	if err != nil {
		return nil, err
	}
	top["mcpServers"] = merged
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// sameServers compares two server maps by their canonical JSON, so key order
// and whitespace inside an entry do not count as a change.
func sameServers(a, b map[string]json.RawMessage) (bool, error) {
	if len(a) != len(b) {
		return false, nil
	}
	for name, raw := range a {
		other, ok := b[name]
		if !ok {
			return false, nil
		}
		ca, err := canonicalJSON(raw)
		if err != nil {
			return false, fmt.Errorf("MCP server %q: %w", name, err)
		}
		cb, err := canonicalJSON(other)
		if err != nil {
			return false, fmt.Errorf("MCP server %q: %w", name, err)
		}
		if !reflect.DeepEqual(ca, cb) {
			return false, nil
		}
	}
	return true, nil
}

// canonicalJSON decodes raw with numbers kept as their literal text, so
// integers beyond float64 precision (and values like 1e400) compare exactly.
func canonicalJSON(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}
