package mcp

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/thlinhit/agent-profiles/internal/fsutil"
)

// maxCredentialsSize caps how much of mcporter's credentials file is read.
const maxCredentialsSize = 16 << 20

// CredentialsPath returns where mcporter keeps the Shared login tokens:
// $XDG_DATA_HOME/mcporter/credentials.json when XDG_DATA_HOME is absolute, else ~/.mcporter/credentials.json.
func CredentialsPath() (string, error) {
	if base := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(base) {
		return filepath.Join(base, "mcporter", "credentials.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".mcporter", "credentials.json"), nil
}

// LoginStatus is what ap knows about one OAuth Pool server's Shared login.
// It is built from credential metadata only; token values are never kept.
type LoginStatus struct {
	LoggedIn bool
	Expires  time.Time // zero if unknown
	Unknown  string    // reason; non-empty means the status is unknown
}

type credentialsFile struct {
	Version *int                       `json:"version"`
	Entries map[string]credentialEntry `json:"entries"`
}

type credentialEntry struct {
	ServerName string          `json:"serverName"`
	Tokens     json.RawMessage `json:"tokens"`
}

type credentialTokens struct {
	AccessToken    string          `json:"access_token"`
	ExpiresAt      json.RawMessage `json:"expires_at"`
	ExpiresAtCamel json.RawMessage `json:"expiresAt"`
}

// ReadLoginStatus reports whether the credentials file at path holds a token for the server name.
// A missing file means not logged in. Symlinks, non-regular files and files over 16 MiB give Unknown.
// Unknown reasons never include file content.
func ReadLoginStatus(path, name string) LoginStatus {
	unknown := func(reason string) LoginStatus { return LoginStatus{Unknown: reason} }
	st, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return LoginStatus{}
	}
	if err != nil {
		return unknown("cannot read credentials file")
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return unknown("credentials file is a symlink")
	}
	if !st.Mode().IsRegular() {
		return unknown("credentials file is not a regular file")
	}
	f, err := os.Open(path)
	if err != nil {
		return unknown("cannot read credentials file")
	}
	defer func() { _ = f.Close() }()
	data, err := fsutil.ReadLimited(f, maxCredentialsSize)
	if errors.Is(err, fsutil.ErrTooLarge) {
		return unknown("credentials file is larger than 16 MiB")
	}
	if err != nil {
		return unknown("cannot read credentials file")
	}
	var file credentialsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return unknown("malformed credentials file")
	}
	if file.Version == nil || *file.Version != 1 {
		return unknown("unsupported credentials file version")
	}
	var status LoginStatus
	var latest float64
	for _, e := range file.Entries {
		if e.ServerName != name || len(e.Tokens) == 0 {
			continue
		}
		var tk credentialTokens
		if err := json.Unmarshal(e.Tokens, &tk); err != nil {
			return unknown("malformed credentials file")
		}
		if len(tk.AccessToken) == 0 {
			continue
		}
		status.LoggedIn = true
		if exp, ok := epochSeconds(tk.ExpiresAt, tk.ExpiresAtCamel); ok && exp > latest {
			latest = exp
		}
	}
	if status.LoggedIn && latest > 0 {
		status.Expires = time.Unix(int64(latest), 0)
	}
	return status
}

// epochSeconds returns the first positive number among the raw values, in epoch seconds.
// A value <= 0 counts as no expiry, because a real token never has one.
func epochSeconds(raws ...json.RawMessage) (float64, bool) {
	for _, raw := range raws {
		if len(raw) == 0 {
			continue
		}
		var v float64
		if json.Unmarshal(raw, &v) == nil && v > 0 {
			return v, true
		}
	}
	return 0, false
}
