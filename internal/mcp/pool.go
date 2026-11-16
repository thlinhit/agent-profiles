// Package mcp loads, saves and validates the MCP pool: the single file of
// Pool servers that every Profile gets.
package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/thlinhit/agent-profiles/internal/fsutil"
)

// maxPoolSize caps how much of an MCP pool file is read.
const maxPoolSize = 1 << 20

// namePattern is the set of names that work as mcporter names and Claude tool names.
var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Proxy describes the proxy an OAuth Pool server goes through.
// An empty Unavailable means the proxy can be used.
type Proxy struct {
	ConfigPath  string
	Unavailable string
}

// Server is one Pool server. Fields it does not know are kept raw in Extra,
// so a save does not drop them.
type Server struct {
	Type    string                     `json:"type,omitempty"`
	Command string                     `json:"command,omitempty"`
	Args    []string                   `json:"args,omitempty"`
	Env     map[string]string          `json:"env,omitempty"`
	URL     string                     `json:"url,omitempty"`
	Headers map[string]string          `json:"headers,omitempty"`
	Auth    string                     `json:"auth,omitempty"`
	Extra   map[string]json.RawMessage `json:"-"`
}

// Pool is the parsed MCP pool file. Top-level keys other than mcpServers are kept in Extra.
type Pool struct {
	Servers map[string]Server
	Extra   map[string]json.RawMessage
}

// UnmarshalJSON decodes only the exact lowercase known keys, each from its own raw value.
// Every other key, including differently cased ones such as "Command", stays raw in Extra.
func (s *Server) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if raw == nil {
		return errors.New("MCP server must be a JSON object")
	}
	var out Server
	decode := func(key string, v any) error {
		b, ok := raw[key]
		if !ok {
			return nil
		}
		delete(raw, key)
		return json.Unmarshal(b, v)
	}
	fields := []struct {
		key string
		v   any
	}{
		{"type", &out.Type},
		{"command", &out.Command},
		{"args", &out.Args},
		{"env", &out.Env},
		{"url", &out.URL},
		{"headers", &out.Headers},
		{"auth", &out.Auth},
	}
	for _, f := range fields {
		if err := decode(f.key, f.v); err != nil {
			return fmt.Errorf("MCP server field %q: %w", f.key, err)
		}
	}
	if len(raw) > 0 {
		out.Extra = raw
	}
	*s = out
	return nil
}

// MarshalJSON writes the known fields that are set, plus Extra, with sorted keys.
func (s Server) MarshalJSON() ([]byte, error) {
	m := make(map[string]json.RawMessage, len(s.Extra)+7)
	for k, v := range s.Extra {
		m[k] = v
	}
	put := func(key string, v any) error {
		b, err := json.Marshal(v)
		if err != nil {
			return err
		}
		m[key] = b
		return nil
	}
	if s.Type != "" {
		if err := put("type", s.Type); err != nil {
			return nil, err
		}
	}
	if s.Command != "" {
		if err := put("command", s.Command); err != nil {
			return nil, err
		}
	}
	if len(s.Args) > 0 {
		if err := put("args", s.Args); err != nil {
			return nil, err
		}
	}
	if len(s.Env) > 0 {
		if err := put("env", s.Env); err != nil {
			return nil, err
		}
	}
	if s.URL != "" {
		if err := put("url", s.URL); err != nil {
			return nil, err
		}
	}
	if len(s.Headers) > 0 {
		if err := put("headers", s.Headers); err != nil {
			return nil, err
		}
	}
	if s.Auth != "" {
		if err := put("auth", s.Auth); err != nil {
			return nil, err
		}
	}
	return json.Marshal(m)
}

// Load reads the pool at path. A missing file gives an empty Pool and exists=false,
// and is never created. Symlinks, non-regular files and files over 1 MiB are refused.
func Load(path string) (Pool, bool, error) {
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return Pool{Servers: map[string]Server{}}, false, nil
	}
	if err != nil {
		return Pool{}, false, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return Pool{}, true, fmt.Errorf("refusing symlink MCP pool %s", path)
	}
	if !st.Mode().IsRegular() {
		return Pool{}, true, fmt.Errorf("refusing non-regular MCP pool file: %s", path)
	}
	if st.Size() > maxPoolSize {
		return Pool{}, true, fmt.Errorf("MCP pool %s is larger than %d bytes", path, maxPoolSize)
	}
	f, err := os.Open(path)
	if err != nil {
		return Pool{}, true, err
	}
	defer func() { _ = f.Close() }()
	data, err := fsutil.ReadLimited(f, maxPoolSize)
	if errors.Is(err, fsutil.ErrTooLarge) {
		return Pool{}, true, fmt.Errorf("MCP pool %s is larger than %d bytes", path, maxPoolSize)
	}
	if err != nil {
		return Pool{}, true, err
	}
	pool, err := decodePool(data)
	if err != nil {
		return Pool{}, true, fmt.Errorf("MCP pool %s: %w", path, err)
	}
	return pool, true, nil
}

func decodePool(data []byte) (Pool, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		return Pool{}, fmt.Errorf("malformed JSON: %w", err)
	}
	if top == nil {
		return Pool{}, errors.New("top level must be a JSON object")
	}
	p := Pool{Servers: map[string]Server{}}
	for k, v := range top {
		if k == "mcpServers" {
			continue
		}
		if p.Extra == nil {
			p.Extra = map[string]json.RawMessage{}
		}
		p.Extra[k] = v
	}
	raw, ok := top["mcpServers"]
	if !ok {
		return p, nil
	}
	if string(bytes.TrimSpace(raw)) == "null" {
		return Pool{}, errors.New("mcpServers must be an object, not null")
	}
	var servers map[string]Server
	if err := json.Unmarshal(raw, &servers); err != nil {
		return Pool{}, fmt.Errorf("mcpServers must be an object: %w", err)
	}
	if servers != nil {
		p.Servers = servers
	}
	return p, nil
}

// Save writes the pool as sorted, indented JSON with a trailing newline.
// If the bytes are unchanged it does not write, so the mtime stays the same.
func Save(path string, p Pool) error {
	data, err := encodePool(p)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if st, err := os.Lstat(path); err == nil && st.Mode().IsRegular() {
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
			return nil
		}
	}
	return fsutil.AtomicWrite(path, data)
}

func encodePool(p Pool) ([]byte, error) {
	servers := p.Servers
	if servers == nil {
		servers = map[string]Server{}
	}
	srv, err := json.Marshal(servers)
	if err != nil {
		return nil, err
	}
	m := make(map[string]json.RawMessage, len(p.Extra)+1)
	for k, v := range p.Extra {
		m[k] = v
	}
	m["mcpServers"] = srv
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

// Validate checks one Pool server definition.
func Validate(name string, s Server) error {
	if !namePattern.MatchString(name) || strings.Contains(name, "__") {
		return fmt.Errorf("invalid MCP server name %q: use 1-64 lowercase letters, digits, '-' or '_', starting with a letter or digit, and no '__'", name)
	}
	switch s.Type {
	case "", "stdio", "http", "sse":
	default:
		return fmt.Errorf("MCP server %q: type must be stdio, http or sse, got %q", name, s.Type)
	}
	if (s.Command == "") == (s.URL == "") {
		return fmt.Errorf("MCP server %q must set exactly one of command or url", name)
	}
	switch s.Auth {
	case "":
	case "oauth":
		if s.URL == "" {
			return fmt.Errorf("MCP server %q: auth oauth requires url", name)
		}
	default:
		return fmt.Errorf("MCP server %q: auth must be empty or \"oauth\", got %q", name, s.Auth)
	}
	return nil
}
