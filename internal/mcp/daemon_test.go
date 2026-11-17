package mcp

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestParseDaemonState(t *testing.T) {
	const running = `{"pid":60981,"protocolVersion":3,"startedAt":1794871161113,"configPath":"","socketPath":"/x/user.sock","generation":"f0a2","views":0,"servers":[],"idleShutdownBlocked":false}`
	cases := []struct {
		name string
		out  string
		want DaemonState
	}{
		{"null", "null\n", DaemonNotRunning},
		{"running object", running, DaemonRunning},
		{"garbage", "mcporter: something else", DaemonUnknown},
		{"empty", "", DaemonUnknown},
		{"pid zero", `{"pid":0}`, DaemonUnknown},
		{"pid negative", `{"pid":-3}`, DaemonUnknown},
		{"pid missing", `{}`, DaemonUnknown},
		{"array", `[]`, DaemonUnknown},
	}
	for _, c := range cases {
		if got := ParseDaemonState(c.out); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

const psOutput = `40423 Mon Nov 17 07:06:00 2026 s011 node /Users/x/.bun/bin/mcporter serve --stdio --servers atlassian
49645 Mon Nov 17 07:13:00 2026 s017 node /Users/x/.bun/bin/mcporter serve --stdio --servers atlassian,jira
60981 Mon Nov 17 06:50:00 2026 ?? node /Users/x/.bun/lib/node_modules/mcporter/dist/cli.js daemon start
70001 Mon Nov 17 06:40:00 2026 s003 vim notes.md
not a ps line
80002 Mon Nov 17 06:41:00 2026 s004 node /Users/x/.bun/bin/mcporter serve --stdio
`

func TestParseLeftoversMatchesServeOnly(t *testing.T) {
	got := ParseLeftovers(psOutput)
	want := []LeftoverProcess{
		{PID: 40423, Started: "07:06", TTY: "s011", Servers: []string{"atlassian"}},
		{PID: 49645, Started: "07:13", TTY: "s017", Servers: []string{"atlassian", "jira"}},
		{PID: 80002, Started: "06:41", TTY: "s004", Servers: nil},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestParseLeftoversNoMatches(t *testing.T) {
	if got := ParseLeftovers("60981 Mon Nov 17 06:50:00 2026 ?? node cli.js daemon start\n"); len(got) != 0 {
		t.Fatalf("got %+v, want none", got)
	}
}

// fakeRun returns a Run func that answers status with status, and start with startErr.
// It records every call as "name args..." under calls.
func fakeRun(calls *[]string, status string, startErr error) func(string, ...string) (string, error) {
	return func(name string, args ...string) (string, error) {
		*calls = append(*calls, strings.Join(append([]string{name}, args...), " "))
		if slices.Contains(args, "start") {
			return "", startErr
		}
		return status, nil
	}
}

func TestDaemonStateUsesStatusCommand(t *testing.T) {
	var calls []string
	d := Daemon{Run: fakeRun(&calls, "null", nil)}
	if got := d.State(); got != DaemonNotRunning {
		t.Fatalf("got %v, want not running", got)
	}
	if want := []string{"mcporter daemon status --json"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q, want %q", calls, want)
	}
	failing := Daemon{Run: func(string, ...string) (string, error) { return "", errors.New("exit status 1") }}
	if got := failing.State(); got != DaemonUnknown {
		t.Fatalf("failing status: got %v, want unknown", got)
	}
}

func TestDaemonStartChecksStatusAfterStart(t *testing.T) {
	var calls []string
	d := Daemon{Run: fakeRun(&calls, `{"pid":7}`, nil)}
	if err := d.Start(); err != nil {
		t.Fatal(err)
	}
	if want := []string{"mcporter daemon start", "mcporter daemon status --json"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("calls = %q, want %q", calls, want)
	}
}

func TestDaemonStartFailsWhenStartFails(t *testing.T) {
	var calls []string
	d := Daemon{Run: fakeRun(&calls, "null", errors.New("exit status 1"))}
	if err := d.Start(); err == nil {
		t.Fatal("want error")
	}
	if len(calls) != 1 {
		t.Fatalf("calls = %q, want only the start", calls)
	}
}

func TestDaemonStartFailsWhenStillNotRunning(t *testing.T) {
	var calls []string
	d := Daemon{Run: fakeRun(&calls, "null", nil)}
	err := d.Start()
	if err == nil || !strings.Contains(err.Error(), "still not running") {
		t.Fatalf("got %v", err)
	}
}

func TestDaemonLeftoversReadsProcessList(t *testing.T) {
	d := Daemon{Processes: func() (string, error) { return psOutput, nil }}
	got, err := d.Leftovers()
	if err != nil || len(got) != 3 {
		t.Fatalf("got %+v, %v", got, err)
	}
	failing := Daemon{Processes: func() (string, error) { return "", errors.New("ps failed") }}
	if _, err := failing.Leftovers(); err == nil {
		t.Fatal("want error when ps fails")
	}
}

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDaemonMetadataPath(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name string
		env  map[string]string
		home string
		want string
	}{
		{"env dir set", map[string]string{"MCPORTER_DAEMON_DIR": dir}, "/home/u", filepath.Join(dir, "daemon", "user.json")},
		{"env dir spaces ignored", map[string]string{"MCPORTER_DAEMON_DIR": "   "}, "/home/u", filepath.Join("/home/u", ".mcporter", "daemon", "user.json")},
		{"env unset", map[string]string{}, "/home/u", filepath.Join("/home/u", ".mcporter", "daemon", "user.json")},
	}
	for _, c := range cases {
		got, err := DaemonMetadataPath(envMap(c.env), c.home)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q, %v; want %q", c.name, got, err, c.want)
		}
	}
	if _, err := DaemonMetadataPath(envMap(nil), ""); err == nil {
		t.Error("no home and no env dir: want error")
	}
}

// staleFixture writes metadata content (nil means no file) and returns a Daemon whose pid check
// reports only alivePID as running.
func staleFixture(t *testing.T, content []byte, alivePID int) (Daemon, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "daemon")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "user.json")
	if content != nil {
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	d := Daemon{
		MetadataPath: func() (string, error) { return path, nil },
		PidRunning:   func(pid int) bool { return pid == alivePID },
	}
	return d, path
}

func TestStaleMetadataDetection(t *testing.T) {
	cases := []struct {
		name    string
		content []byte
		alive   int
		want    bool
	}{
		{"pid dead", []byte(`{"pid":60981}`), 0, true},
		{"pid alive", []byte(`{"pid":60981}`), 60981, false},
		{"garbage JSON", []byte(`{not json`), 0, false},
		{"pid zero", []byte(`{"pid":0}`), 0, false},
		{"pid negative", []byte(`{"pid":-4}`), 0, false},
		{"pid not a number", []byte(`{"pid":"60981"}`), 0, false},
		{"pid missing", []byte(`{}`), 0, false},
	}
	for _, c := range cases {
		d, _ := staleFixture(t, c.content, c.alive)
		_, got, err := d.StaleMetadata()
		if err != nil || got != c.want {
			t.Errorf("%s: got stale=%v err=%v, want %v", c.name, got, err, c.want)
		}
	}
	d, _ := staleFixture(t, nil, 0)
	if _, got, err := d.StaleMetadata(); got || err != nil {
		t.Errorf("missing file: got stale=%v err=%v", got, err)
	}
}

func TestStaleMetadataRefusesSymlink(t *testing.T) {
	d, path := staleFixture(t, nil, 0)
	target := filepath.Join(t.TempDir(), "real.json")
	if err := os.WriteFile(target, []byte(`{"pid":60981}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, got, err := d.StaleMetadata(); got || err == nil {
		t.Fatalf("symlink: got stale=%v err=%v, want error and not stale", got, err)
	}
}

func TestNilInjectedFuncsAreNotStale(t *testing.T) {
	if _, got, err := (Daemon{}).StaleMetadata(); got || err != nil {
		t.Fatalf("got stale=%v err=%v", got, err)
	}
}

func TestStaleClearMovesMetadataAsideAndLeavesSocket(t *testing.T) {
	d, path := staleFixture(t, []byte(`{"pid":60981}`), 0)
	sock := filepath.Join(filepath.Dir(path), "user.sock")
	if err := os.WriteFile(sock, []byte("sock"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, stale, err := d.StaleMetadata()
	if err != nil || !stale {
		t.Fatalf("setup: stale=%v err=%v", stale, err)
	}
	moved, err := s.Clear()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.Base(filepath.Dir(moved)), "user.json.stale-60981-") || filepath.Base(moved) != "user.json" {
		t.Fatalf("moved to %s, want user.json.stale-60981-*/user.json", moved)
	}
	if b, err := os.ReadFile(moved); err != nil || string(b) != `{"pid":60981}` {
		t.Fatalf("moved file: %q, %v", b, err)
	}
	if _, err := os.Lstat(path); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("metadata still at %s (err %v)", path, err)
	}
	if b, err := os.ReadFile(sock); err != nil || string(b) != "sock" {
		t.Fatalf("socket touched: %q, %v", b, err)
	}
}

// A file already named user.json.stale-<pid> is not part of the move and is never replaced.
func TestStaleClearLeavesOldNameFileUntouched(t *testing.T) {
	d, path := staleFixture(t, []byte(`{"pid":7}`), 0)
	old := path + ".stale-7"
	if err := os.WriteFile(old, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	s, _, _ := d.StaleMetadata()
	moved, err := s.Clear()
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(old); err != nil || string(b) != "keep" {
		t.Fatalf("old-name file changed: %q, %v", b, err)
	}
	if _, err := os.Stat(moved); err != nil {
		t.Fatal(err)
	}
}

// Another login replaces user.json with a live daemon's file before the move: the file is moved back,
// the socket is left alone, and Clear errors.
func TestStaleClearReplacedBeforeMoveIsMovedBack(t *testing.T) {
	d, path := staleFixture(t, []byte(`{"pid":60981}`), 0)
	d.PidRunning = func(pid int) bool { return pid == 99999 }
	sock := filepath.Join(filepath.Dir(path), "user.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s, stale, err := d.StaleMetadata()
	if err != nil || !stale {
		t.Fatalf("setup: stale=%v err=%v", stale, err)
	}
	s.afterAsideDir = func(string) {
		if err := os.WriteFile(path, []byte(`{"pid":99999}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := s.Clear()
	if err == nil || !strings.Contains(err.Error(), "started during recovery") {
		t.Fatalf("got %v, want started-during-recovery error", err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != `{"pid":99999}` {
		t.Fatalf("live metadata not back at %s: %q, %v", path, b, err)
	}
	if _, err := os.Lstat(moved); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("moved copy still present (err %v)", err)
	}
	if _, err := os.Lstat(sock); err != nil {
		t.Fatalf("socket removed: %v", err)
	}
}

// A new user.json appears after the move and before move-back: the moved file stays in its aside
// directory and the error names that directory.
func TestStaleClearNewMetadataAtMoveBackLeavesMovedFile(t *testing.T) {
	d, path := staleFixture(t, []byte(`{"pid":60981}`), 0)
	d.PidRunning = func(pid int) bool { return pid == 99999 }
	sock := filepath.Join(filepath.Dir(path), "user.sock")
	if err := os.WriteFile(sock, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s, _, _ := d.StaleMetadata()
	s.afterAsideDir = func(string) {
		if err := os.WriteFile(path, []byte(`{"pid":99999}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	s.afterMove = func() {
		if err := os.WriteFile(path, []byte(`{"pid":88888}`), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	moved, err := s.Clear()
	if err == nil || !strings.Contains(err.Error(), filepath.Dir(moved)) {
		t.Fatalf("got %v, want error naming %s", err, filepath.Dir(moved))
	}
	if b, err := os.ReadFile(moved); err != nil || string(b) != `{"pid":99999}` {
		t.Fatalf("moved file not left in place: %q, %v", b, err)
	}
	if b, err := os.ReadFile(path); err != nil || string(b) != `{"pid":88888}` {
		t.Fatalf("new metadata changed: %q, %v", b, err)
	}
	if _, err := os.Lstat(sock); err != nil {
		t.Fatalf("socket removed: %v", err)
	}
}

func TestWaitExitedPollsUntilGone(t *testing.T) {
	var slept time.Duration
	alive := map[int]bool{5: true}
	d := Daemon{
		PidRunning: func(pid int) bool { return alive[pid] },
		Sleep: func(dur time.Duration) {
			slept += dur
			alive[5] = false // exits after the first poll
		},
	}
	if err := d.WaitExited([]int{5}); err != nil {
		t.Fatal(err)
	}
	if slept != 100*time.Millisecond {
		t.Fatalf("slept %v, want one 100ms poll", slept)
	}
}

func TestWaitExitedGivesUpAfterFiveSeconds(t *testing.T) {
	var slept time.Duration
	d := Daemon{
		PidRunning: func(int) bool { return true },
		Sleep:      func(dur time.Duration) { slept += dur },
	}
	err := d.WaitExited([]int{40423, 49645})
	if err == nil || !strings.Contains(err.Error(), "40423") {
		t.Fatalf("got %v, want error naming pid 40423", err)
	}
	if slept != 5*time.Second {
		t.Fatalf("slept %v, want 5s", slept)
	}
}
