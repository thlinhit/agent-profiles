package mcp

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
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
