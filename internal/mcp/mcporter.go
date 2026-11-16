package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/thlinhit/agent-profiles/internal/fsutil"
)

const (
	// mcporterCheckTimeout bounds each version command run by SystemAvailable.
	mcporterCheckTimeout = 5 * time.Second
	// mcporterMinNode is the lowest Node major version that mcporter's OAuth serve supports.
	mcporterMinNode = 24
)

// mcporterMinVersion is the lowest mcporter that has serve, MCPORTER_OAUTH_NO_BROWSER and the refresh lock.
var mcporterMinVersion = [3]int{0, 17, 0}

// versionPattern finds x.y.z in command output, tolerating a leading "v" and trailing text.
var versionPattern = regexp.MustCompile(`v?(\d+)\.(\d+)\.(\d+)`)

// McporterConfigPath returns where ap writes its mcporter config:
// $XDG_DATA_HOME/agent-profiles/mcporter.json, or ~/.local/share when XDG_DATA_HOME is unset.
func McporterConfigPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("XDG_DATA_HOME must be absolute")
	}
	return filepath.Join(base, "agent-profiles", "mcporter.json"), nil
}

type mcporterEntry struct {
	BaseURL   string `json:"baseUrl"`
	Lifecycle string `json:"lifecycle"`
}

type mcporterFile struct {
	McpServers map[string]mcporterEntry `json:"mcpServers"`
	// Imports is always empty. Without it mcporter also loads Cursor, Claude
	// and Codex configs from the home directory into the served view.
	Imports []string `json:"imports"`
}

// WriteMcporterConfig regenerates the mcporter config at path from the OAuth Pool servers.
// It returns changed=false and writes nothing when the bytes are already identical.
func WriteMcporterConfig(path string, pool Pool) (changed bool, err error) {
	servers := map[string]mcporterEntry{}
	for name, s := range pool.Servers {
		if s.Auth == "oauth" {
			servers[name] = mcporterEntry{BaseURL: s.URL, Lifecycle: "keep-alive"}
		}
	}
	data, err := json.MarshalIndent(mcporterFile{McpServers: servers, Imports: []string{}}, "", "  ")
	if err != nil {
		return false, err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if st, err := os.Lstat(path); err == nil && st.Mode().IsRegular() {
		if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, data) {
			return false, nil
		}
	}
	if err := fsutil.AtomicWrite(path, data); err != nil {
		return false, err
	}
	return true, nil
}

// Available reports why the mcporter OAuth path cannot be used, or "" when it can.
// lookPath and run are injected so callers and tests never run real binaries.
func Available(lookPath func(string) (string, error), run func(name string, args ...string) (string, error)) string {
	const unknown = "mcporter version unknown (needs mcporter >= 0.17.0 and Node >= 24)"
	if _, err := lookPath("mcporter"); err != nil {
		return "mcporter not found (needs mcporter >= 0.17.0 and Node >= 24)"
	}
	out, err := run("mcporter", "--version")
	if err != nil {
		return unknown
	}
	v, ok := parseVersion(out)
	if !ok {
		return unknown
	}
	if compareVersion(v, mcporterMinVersion) < 0 {
		return fmt.Sprintf("mcporter %d.%d.%d is too old (needs >= 0.17.0)", v[0], v[1], v[2])
	}
	out, err = run("node", "--version")
	if err != nil {
		return "node not found (needs Node >= 24)"
	}
	v, ok = parseVersion(out)
	if !ok {
		return "node version unknown (needs Node >= 24)"
	}
	if v[0] < mcporterMinNode {
		return fmt.Sprintf("node v%d.%d.%d is too old (needs >= 24)", v[0], v[1], v[2])
	}
	return ""
}

// SystemAvailable runs Available against the real PATH, with a 5s timeout per command.
func SystemAvailable() string {
	return Available(exec.LookPath, func(name string, args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), mcporterCheckTimeout)
		defer cancel()
		out, err := exec.CommandContext(ctx, name, args...).Output()
		return string(out), err
	})
}

// parseVersion reads the first x.y.z in s.
func parseVersion(s string) ([3]int, bool) {
	m := versionPattern.FindStringSubmatch(s)
	if m == nil {
		return [3]int{}, false
	}
	var v [3]int
	for i := range v {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return [3]int{}, false
		}
		v[i] = n
	}
	return v, true
}

// compareVersion returns -1, 0 or 1 comparing a with b numerically, part by part.
func compareVersion(a, b [3]int) int {
	for i := range a {
		switch {
		case a[i] < b[i]:
			return -1
		case a[i] > b[i]:
			return 1
		}
	}
	return 0
}
