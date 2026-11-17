package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	// daemonStatusTimeout bounds the daemon status command and the process listing.
	daemonStatusTimeout = 5 * time.Second
	// daemonStartTimeout bounds "mcporter daemon start", which can take up to 45s before it fails.
	daemonStartTimeout = 60 * time.Second
)

// DaemonState is what "mcporter daemon status --json" says about the mcporter daemon.
type DaemonState int

const (
	// DaemonNotRunning means the status printed null.
	DaemonNotRunning DaemonState = iota
	// DaemonRunning means the status printed an object with a positive pid.
	DaemonRunning
	// DaemonUnknown means the status could not be read.
	DaemonUnknown
)

// ParseDaemonState reads the output of "mcporter daemon status --json".
func ParseDaemonState(out string) DaemonState {
	s := strings.TrimSpace(out)
	if s == "null" {
		return DaemonNotRunning
	}
	var st struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal([]byte(s), &st); err != nil || st.PID <= 0 {
		return DaemonUnknown
	}
	return DaemonRunning
}

// LeftoverProcess is a "mcporter serve" process, usually from an old session, that holds Pool servers.
type LeftoverProcess struct {
	PID     int
	Started string // HH:MM
	TTY     string
	Servers []string
}

// ParseLeftovers reads "ps -axo pid=,lstart=,tty=,command=" output and keeps the mcporter serve processes.
// The mcporter daemon and unrelated lines are ignored.
func ParseLeftovers(out string) []LeftoverProcess {
	var procs []LeftoverProcess
	for _, line := range strings.Split(out, "\n") {
		// pid, then lstart as five fields ("Tue Nov 17 07:06:00 2026"), then tty, then the command.
		f := strings.Fields(line)
		if len(f) < 8 {
			continue
		}
		pid, err := strconv.Atoi(f[0])
		if err != nil || pid <= 0 {
			continue
		}
		cmd := f[7:]
		if !strings.Contains(strings.Join(cmd, " "), "mcporter serve") {
			continue
		}
		started := f[4]
		if len(started) >= 5 {
			started = started[:5]
		}
		procs = append(procs, LeftoverProcess{PID: pid, Started: started, TTY: f[6], Servers: serversArg(cmd)})
	}
	return procs
}

// serversArg returns the comma-separated values of the --servers argument, or nil.
func serversArg(cmd []string) []string {
	for i := 0; i+1 < len(cmd); i++ {
		if cmd[i] == "--servers" {
			return strings.FieldsFunc(cmd[i+1], func(r rune) bool { return r == ',' })
		}
	}
	return nil
}

// Daemon reaches the mcporter daemon through injected funcs, so tests never run mcporter, ps or kill.
type Daemon struct {
	// Run runs a mcporter command and returns its stdout.
	Run func(name string, args ...string) (string, error)
	// Processes returns "ps -axo pid=,lstart=,tty=,command=" output.
	Processes func() (string, error)
	// Signal sends SIGTERM to pid.
	Signal func(pid int) error
}

// SystemDaemon uses the real mcporter, ps and kill, with MCPORTER_CONFIG set to configPath.
// Each command is bounded by a timeout.
func SystemDaemon(configPath string) Daemon {
	return Daemon{
		Run: func(name string, args ...string) (string, error) {
			timeout := daemonStatusTimeout
			if slices.Contains(args, "start") {
				timeout = daemonStartTimeout
			}
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			cmd := exec.CommandContext(ctx, name, args...)
			cmd.Env = append(os.Environ(), "MCPORTER_CONFIG="+configPath)
			out, err := cmd.Output()
			return string(out), err
		},
		Processes: func() (string, error) {
			ctx, cancel := context.WithTimeout(context.Background(), daemonStatusTimeout)
			defer cancel()
			out, err := exec.CommandContext(ctx, "ps", "-axo", "pid=,lstart=,tty=,command=").Output()
			return string(out), err
		},
		Signal: func(pid int) error { return syscall.Kill(pid, syscall.SIGTERM) },
	}
}

// State asks the daemon for its status. A failing status command counts as DaemonUnknown.
func (d Daemon) State() DaemonState {
	out, err := d.Run("mcporter", "daemon", "status", "--json")
	if err != nil {
		return DaemonUnknown
	}
	return ParseDaemonState(out)
}

// Leftovers lists the leftover mcporter serve processes.
func (d Daemon) Leftovers() ([]LeftoverProcess, error) {
	out, err := d.Processes()
	if err != nil {
		return nil, err
	}
	return ParseLeftovers(out), nil
}

// Start starts the daemon and checks its status again.
func (d Daemon) Start() error {
	if _, err := d.Run("mcporter", "daemon", "start"); err != nil {
		return err
	}
	if d.State() != DaemonRunning {
		return errors.New("mcporter daemon is still not running after start")
	}
	return nil
}
