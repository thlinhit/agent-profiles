package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func statusTestCreds(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestStatusCredentialsPathXDG(t *testing.T) {
	base := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", base)
	got, err := CredentialsPath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(base, "mcporter", "credentials.json"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStatusCredentialsPathFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, xdg := range []string{"", "relative/dir"} {
		t.Setenv("XDG_DATA_HOME", xdg)
		got, err := CredentialsPath()
		if err != nil {
			t.Fatal(err)
		}
		if want := filepath.Join(home, ".mcporter", "credentials.json"); got != want {
			t.Fatalf("XDG_DATA_HOME=%q: got %q, want %q", xdg, got, want)
		}
	}
}

func TestStatusMissingFileNotLoggedIn(t *testing.T) {
	st := ReadLoginStatus(filepath.Join(t.TempDir(), "absent.json"), "atlassian")
	if st.LoggedIn || st.Unknown != "" || !st.Expires.IsZero() {
		t.Fatalf("got %+v, want not logged in", st)
	}
}

func TestStatusLoggedInWithExpiry(t *testing.T) {
	path := statusTestCreds(t, `{"version":1,"entries":{
		"a":{"serverName":"atlassian","tokens":{"access_token":"fake-token","expires_at":1796200000}},
		"b":{"serverName":"atlassian","tokens":{"access_token":"fake-token-2","expires_at":1796300000}},
		"c":{"serverName":"jira","tokens":{"access_token":"fake-token-3","expires_at":1799999999}}}}`)
	st := ReadLoginStatus(path, "atlassian")
	if !st.LoggedIn || st.Unknown != "" {
		t.Fatalf("got %+v, want logged in", st)
	}
	if want := time.Unix(1796300000, 0); !st.Expires.Equal(want) {
		t.Fatalf("Expires = %v, want the largest atlassian expires_at %v", st.Expires, want)
	}
}

func TestStatusExpiresAtCamelCase(t *testing.T) {
	path := statusTestCreds(t, `{"version":1,"entries":{"a":{"serverName":"atlassian","tokens":{"access_token":"fake-token","expiresAt":1796200000}}}}`)
	st := ReadLoginStatus(path, "atlassian")
	if !st.LoggedIn || !st.Expires.Equal(time.Unix(1796200000, 0)) {
		t.Fatalf("got %+v", st)
	}
}

func TestStatusMissingExpiresAtIsLoggedInWithoutExpiry(t *testing.T) {
	path := statusTestCreds(t, `{"version":1,"entries":{"a":{"serverName":"atlassian","tokens":{"access_token":"fake-token"}}}}`)
	st := ReadLoginStatus(path, "atlassian")
	if !st.LoggedIn || st.Unknown != "" || !st.Expires.IsZero() {
		t.Fatalf("got %+v, want logged in with zero expiry", st)
	}
}

func TestStatusNonPositiveExpiresAtIsNoExpiry(t *testing.T) {
	for _, v := range []string{"0", "-5"} {
		path := statusTestCreds(t, `{"version":1,"entries":{"a":{"serverName":"atlassian","tokens":{"access_token":"fake-token","expires_at":`+v+`}}}}`)
		st := ReadLoginStatus(path, "atlassian")
		if !st.LoggedIn || st.Unknown != "" || !st.Expires.IsZero() {
			t.Fatalf("expires_at %s: got %+v, want logged in with zero expiry", v, st)
		}
	}
}

func TestStatusEmptyTokenOrOtherServerNotLoggedIn(t *testing.T) {
	path := statusTestCreds(t, `{"version":1,"entries":{
		"a":{"serverName":"atlassian","tokens":{"access_token":"","expires_at":1796200000}},
		"b":{"serverName":"atlassian","tokens":null},
		"c":{"serverName":"jira","tokens":{"access_token":"fake-token","expires_at":1796200000}}}}`)
	st := ReadLoginStatus(path, "atlassian")
	if st.LoggedIn || st.Unknown != "" {
		t.Fatalf("got %+v, want not logged in", st)
	}
}

func TestStatusMalformedIsUnknownWithoutContent(t *testing.T) {
	path := statusTestCreds(t, `{"version":1,"entries":{"a":{"serverName":"atlassian","tokens":{"access_token":"fake-token",`)
	st := ReadLoginStatus(path, "atlassian")
	if st.Unknown == "" || st.LoggedIn {
		t.Fatalf("got %+v, want unknown", st)
	}
	if strings.Contains(st.Unknown, "fake-token") {
		t.Fatal("unknown reason leaks file content")
	}
}

func TestStatusUnknownVersionIsUnknown(t *testing.T) {
	path := statusTestCreds(t, `{"version":2,"entries":{}}`)
	if st := ReadLoginStatus(path, "atlassian"); st.Unknown == "" {
		t.Fatalf("got %+v, want unknown", st)
	}
}

func TestStatusSymlinkIsUnknown(t *testing.T) {
	target := statusTestCreds(t, `{"version":1,"entries":{"a":{"serverName":"atlassian","tokens":{"access_token":"fake-token"}}}}`)
	link := filepath.Join(t.TempDir(), "credentials.json")
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	st := ReadLoginStatus(link, "atlassian")
	if st.Unknown == "" || st.LoggedIn {
		t.Fatalf("got %+v, want unknown", st)
	}
}

func TestStatusNonRegularIsUnknown(t *testing.T) {
	if st := ReadLoginStatus(t.TempDir(), "atlassian"); st.Unknown == "" {
		t.Fatalf("got %+v, want unknown for a directory", st)
	}
}

func TestStatusTooLargeIsUnknown(t *testing.T) {
	path := filepath.Join(t.TempDir(), "credentials.json")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(maxCredentialsSize + 1); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if st := ReadLoginStatus(path, "atlassian"); st.Unknown == "" {
		t.Fatalf("got %+v, want unknown", st)
	}
}
