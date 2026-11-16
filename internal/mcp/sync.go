package mcp

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/thlinhit/agent-profiles/internal/fsutil"
)

const (
	// ledgerFile lists the Managed servers of one Profile, as a sorted JSON array.
	ledgerFile = ".ap-mcp.json"
	// backupFile keeps the hand-made servers that a Pool server replaced.
	backupFile = ".ap-mcp-backup.json"
	// backupVersion is the only backup format that is read or written.
	backupVersion = 1
	// maxSidecarSize caps reads of the Agent config, the ledger and the backup.
	maxSidecarSize = 16 << 20
	// replacedText ends the warning printed when a hand-made server is replaced.
	replacedText = "replaced by pool; backup saved"
)

// Report lists what Sync did to one Profile: one line per server, sorted by name.
// A line is "+ name" (added), "~ name" (changed), "- name" (removed by the pool),
// "! name replaced by pool; backup saved", or "! name reason" (a skipped Pool server).
type Report struct {
	Lines []string
}

// Changed reports whether Sync changed the Profile's servers on this run.
// Skip warnings do not count as a change.
func (r Report) Changed() bool {
	for _, line := range r.Lines {
		if !strings.HasPrefix(line, "! ") || strings.HasSuffix(line, replacedText) {
			return true
		}
	}
	return false
}

// syncLine is one report line with the server name it is sorted by.
type syncLine struct {
	name string
	text string
}

// backupDoc is the on-disk form of the backup file. Each name keeps every
// distinct hand-made definition that a Pool server replaced, in order.
type backupDoc struct {
	Version int                          `json:"version"`
	Servers map[string][]json.RawMessage `json:"servers"`
}

// Sync makes the Managed servers of one Profile match pool.
//
// It reads the Agent config, the ledger and the backup first, so a malformed
// file fails before anything is written. Then it translates every Pool server
// with s. A Pool server that cannot run here is skipped with a warning, and if
// it is Managed already its entry stays. A Managed server that the pool no
// longer defines is removed. A hand-made server with a Pool server's name is
// backed up and replaced. Writes happen in this order: backup, Agent config,
// ledger. Each file is written only when its bytes change.
func Sync(s Setter, profileDir string, pool Pool, proxy Proxy) (Report, error) {
	agentPath := s.MCPConfigFile(profileDir)
	if filepath.Dir(agentPath) != filepath.Clean(profileDir) {
		return Report{}, fmt.Errorf("MCP config %s is not directly inside %s", agentPath, profileDir)
	}
	agentName := filepath.Base(agentPath)

	root, err := os.OpenRoot(profileDir)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = root.Close() }()

	current, err := readSidecar(root, agentName)
	if err != nil {
		return Report{}, err
	}
	existing, err := agentServers(current)
	if err != nil {
		return Report{}, fmt.Errorf("%s: %w", agentName, err)
	}
	ledgerBytes, err := readSidecar(root, ledgerFile)
	if err != nil {
		return Report{}, err
	}
	ledger, err := parseLedger(ledgerBytes)
	if err != nil {
		return Report{}, err
	}
	backupBytes, err := readSidecar(root, backupFile)
	if err != nil {
		return Report{}, err
	}
	backup, err := parseBackup(backupBytes)
	if err != nil {
		return Report{}, err
	}

	managed := make(map[string]bool, len(ledger))
	for _, name := range ledger {
		managed[name] = true
	}

	var lines []syncLine
	next := map[string]json.RawMessage{}
	var kept []string // Managed servers whose Pool server cannot run here now
	for _, name := range slices.Sorted(maps.Keys(pool.Servers)) {
		entry, skip, err := s.Translate(name, pool.Servers[name], proxy)
		if err != nil {
			return Report{}, fmt.Errorf("MCP server %q: %w", name, err)
		}
		if skip != "" {
			lines = append(lines, syncLine{name, "! " + name + " " + skip})
			if managed[name] {
				kept = append(kept, name)
			}
			continue
		}
		next[name] = entry
	}

	var removed []string
	for _, name := range ledger {
		if _, ok := pool.Servers[name]; !ok {
			removed = append(removed, name)
			lines = append(lines, syncLine{name, "- " + name})
		}
	}

	backupChanged := false
	for _, name := range slices.Sorted(maps.Keys(next)) {
		cur, present := existing[name]
		if !present {
			lines = append(lines, syncLine{name, "+ " + name})
			continue
		}
		if managed[name] {
			same, err := sameJSON(cur, next[name])
			if err != nil {
				return Report{}, fmt.Errorf("MCP server %q: %w", name, err)
			}
			if !same {
				lines = append(lines, syncLine{name, "~ " + name})
			}
			continue
		}
		// A hand-made server with a Pool server's name is a clash, even when its
		// definition is identical. Keep its definition before it is replaced or adopted.
		added, err := backup.add(name, cur)
		if err != nil {
			return Report{}, fmt.Errorf("MCP server %q: %w", name, err)
		}
		backupChanged = backupChanged || added
		lines = append(lines, syncLine{name, "! " + name + " " + replacedText})
	}

	if backupChanged {
		data, err := json.MarshalIndent(backup, "", "  ")
		if err != nil {
			return Report{}, err
		}
		if err := fsutil.AtomicWriteRoot(root, backupFile, append(data, '\n')); err != nil {
			return Report{}, err
		}
	}

	updated, err := s.SetServers(current, removed, next)
	if err != nil {
		return Report{}, fmt.Errorf("%s: %w", agentName, err)
	}
	if !bytes.Equal(updated, current) {
		if err := fsutil.AtomicWriteRoot(root, agentName, updated); err != nil {
			return Report{}, err
		}
	}

	names := make([]string, 0, len(next)+len(kept))
	for name := range next {
		names = append(names, name)
	}
	names = append(names, kept...)
	slices.Sort(names)
	names = slices.Compact(names)
	ledgerData, err := json.Marshal(names)
	if err != nil {
		return Report{}, err
	}
	oldLedger := ledgerBytes
	if oldLedger == nil {
		oldLedger = []byte("[]")
	}
	if !bytes.Equal(oldLedger, ledgerData) {
		if err := fsutil.AtomicWriteRoot(root, ledgerFile, ledgerData); err != nil {
			return Report{}, err
		}
	}

	slices.SortStableFunc(lines, func(a, b syncLine) int { return strings.Compare(a.name, b.name) })
	var out []string
	for _, l := range lines {
		out = append(out, l.text)
	}
	return Report{Lines: out}, nil
}

// readSidecar reads a regular file directly inside root. A missing file gives nil.
// Symlinks and non-regular files are refused, and so are files over maxSidecarSize.
func readSidecar(root *os.Root, name string) ([]byte, error) {
	st, err := root.Lstat(name)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing symlink %s", name)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("refusing non-regular file %s", name)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	data, err := fsutil.ReadLimited(f, maxSidecarSize)
	if errors.Is(err, fsutil.ErrTooLarge) {
		return nil, fmt.Errorf("%s is larger than %d bytes", name, maxSidecarSize)
	}
	if err != nil {
		return nil, err
	}
	return data, nil
}

// agentServers returns the top-level mcpServers of an Agent config. Empty input gives no servers.
func agentServers(current []byte) (map[string]json.RawMessage, error) {
	servers := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(current)) == 0 {
		return servers, nil
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(current, &top); err != nil || top == nil {
		return nil, errors.New("parse MCP config: not a JSON object")
	}
	raw, ok := top["mcpServers"]
	if !ok {
		return servers, nil
	}
	if err := json.Unmarshal(raw, &servers); err != nil || servers == nil {
		return nil, errors.New("parse MCP config: mcpServers is not a JSON object")
	}
	return servers, nil
}

// parseLedger reads the sorted list of Managed server names. Missing input gives none.
func parseLedger(data []byte) ([]string, error) {
	if data == nil {
		return nil, nil
	}
	bad := fmt.Errorf("MCP ledger %s must be a JSON array of non-empty strings", ledgerFile)
	var elems []json.RawMessage
	if err := json.Unmarshal(data, &elems); err != nil || elems == nil {
		return nil, bad
	}
	names := make([]string, 0, len(elems))
	for _, e := range elems {
		if !isJSONString(e) {
			return nil, bad
		}
		var name string
		if err := json.Unmarshal(e, &name); err != nil || name == "" {
			return nil, bad
		}
		names = append(names, name)
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

// isJSONString reports whether raw is a JSON string literal (after leading space), not null or another value.
func isJSONString(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '"'
}

// isJSONArray reports whether raw is a JSON array literal (after leading space), not null or another value.
func isJSONArray(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '['
}

// parseBackup reads the backup. A missing file gives an empty backup. Malformed
// input or an unknown version is an error, never a reset.
func parseBackup(data []byte) (*backupDoc, error) {
	if data == nil {
		return &backupDoc{Version: backupVersion, Servers: map[string][]json.RawMessage{}}, nil
	}
	var top struct {
		Version *int                       `json:"version"`
		Servers map[string]json.RawMessage `json:"servers"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		return nil, fmt.Errorf("MCP backup %s is malformed: %w", backupFile, err)
	}
	if top.Version == nil {
		return nil, fmt.Errorf("MCP backup %s has no version", backupFile)
	}
	if *top.Version != backupVersion {
		return nil, fmt.Errorf("MCP backup %s has unsupported version %d", backupFile, *top.Version)
	}
	if top.Servers == nil {
		return nil, fmt.Errorf("MCP backup %s: servers must be a JSON object", backupFile)
	}
	b := backupDoc{Version: backupVersion, Servers: make(map[string][]json.RawMessage, len(top.Servers))}
	for name, raw := range top.Servers {
		if !isJSONArray(raw) {
			return nil, fmt.Errorf("MCP backup %s: servers %q must be a JSON array", backupFile, name)
		}
		var defs []json.RawMessage
		if err := json.Unmarshal(raw, &defs); err != nil {
			return nil, fmt.Errorf("MCP backup %s: servers %q: %w", backupFile, name, err)
		}
		b.Servers[name] = defs
	}
	return &b, nil
}

// add appends def under name unless a byte-equal (after compacting) definition is already stored.
// It reports whether anything was appended.
func (b *backupDoc) add(name string, def json.RawMessage) (bool, error) {
	c, err := compactJSON(def)
	if err != nil {
		return false, err
	}
	for _, have := range b.Servers[name] {
		hc, err := compactJSON(have)
		if err != nil {
			return false, err
		}
		if bytes.Equal(hc, c) {
			return false, nil
		}
	}
	b.Servers[name] = append(b.Servers[name], json.RawMessage(c))
	return true, nil
}

// sameJSON reports whether two JSON values are equal after compacting, so whitespace does not count.
func sameJSON(a, b json.RawMessage) (bool, error) {
	ca, err := compactJSON(a)
	if err != nil {
		return false, err
	}
	cb, err := compactJSON(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ca, cb), nil
}

func compactJSON(raw []byte) ([]byte, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
