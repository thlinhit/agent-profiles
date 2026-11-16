package mcp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
)

// MergeServers writes next into the top-level mcpServers of current, drops prev
// names that are not in next, and leaves every other key and server untouched.
// When the resulting mcpServers equals the current one, current is returned unchanged.
// It is shared by the Adapters whose config keeps MCP servers under mcpServers.
func MergeServers(current []byte, prev []string, next map[string]json.RawMessage) ([]byte, error) {
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
