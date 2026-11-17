package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
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
	// MetadataPath returns the daemon metadata file (user.json). A nil func means there is none.
	MetadataPath func() (string, error)
	// PidRunning reports whether pid is a live process. A nil func counts every pid as running, so nothing is cleared.
	PidRunning func(pid int) bool
	// Sleep waits between polls in WaitExited. A nil func does not wait.
	Sleep func(time.Duration)
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
		Sleep:  time.Sleep,
		MetadataPath: func() (string, error) {
			home, _ := os.UserHomeDir()
			return DaemonMetadataPath(os.Getenv, home)
		},
		PidRunning: func(pid int) bool {
			err := syscall.Kill(pid, 0)
			return err == nil || errors.Is(err, syscall.EPERM)
		},
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

// DaemonMetadataPath is the daemon metadata file. MCPORTER_DAEMON_DIR (trimmed, made absolute) holds a
// daemon/ directory; otherwise the file is under <home>/.mcporter.
func DaemonMetadataPath(getenv func(string) string, home string) (string, error) {
	if dir := strings.TrimSpace(getenv("MCPORTER_DAEMON_DIR")); dir != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		return filepath.Join(abs, "daemon", "user.json"), nil
	}
	if home == "" {
		return "", errors.New("cannot find the home directory for the mcporter daemon metadata")
	}
	return filepath.Join(home, ".mcporter", "daemon", "user.json"), nil
}

// StaleDaemon is daemon metadata that names a pid which is no longer running.
type StaleDaemon struct {
	Path string
	PID  int
	// alive is the pid check used when the metadata was read.
	alive func(int) bool
	// Test hooks, nil in production: afterAsideDir runs after the fresh directory is made, afterMove after
	// the rename and before the moved file is read again.
	afterAsideDir func(aside string)
	afterMove     func()
}

// StaleMetadata reports whether the daemon metadata names a dead pid. Missing, unreadable or invalid
// metadata, and a live pid, are not stale. Nothing is changed. A symlinked metadata file is an error.
// Call it only when the daemon is not running.
func (d Daemon) StaleMetadata() (StaleDaemon, bool, error) {
	if d.MetadataPath == nil || d.PidRunning == nil {
		return StaleDaemon{}, false, nil
	}
	path, err := d.MetadataPath()
	if err != nil {
		return StaleDaemon{}, false, nil
	}
	info, err := os.Lstat(path)
	if err != nil {
		return StaleDaemon{}, false, nil
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return StaleDaemon{}, false, fmt.Errorf("refusing to use mcporter daemon metadata %s: it is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return StaleDaemon{}, false, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return StaleDaemon{}, false, nil
	}
	var md struct {
		PID int `json:"pid"`
	}
	if json.Unmarshal(data, &md) != nil || md.PID <= 0 {
		return StaleDaemon{}, false, nil
	}
	if d.PidRunning(md.PID) {
		return StaleDaemon{}, false, nil
	}
	return StaleDaemon{Path: path, PID: md.PID, alive: d.PidRunning}, true, nil
}

// Clear moves the daemon metadata into a new directory beside it, user.json.stale-<pid>-*/user.json. The
// move is one os.Rename into a directory made just before, so an existing file is never replaced. The moved
// file is read again. If it is no longer the stale pid, or that pid now runs, the file is linked back and
// Clear returns an error. If a new user.json already exists at move-back, the file stays in the new
// directory and the error names it. The socket is never touched: mcporter removes a leftover socket itself
// when it starts without metadata. It returns the path of the moved file.
func (s StaleDaemon) Clear() (string, error) {
	if err := refuseSymlink(s.Path); err != nil {
		return "", err
	}
	base := filepath.Base(s.Path)
	aside, err := os.MkdirTemp(filepath.Dir(s.Path), fmt.Sprintf("%s.stale-%d-", base, s.PID))
	if err != nil {
		return "", fmt.Errorf("make a directory for the stale daemon metadata: %w", err)
	}
	moved := filepath.Join(aside, base)
	if s.afterAsideDir != nil {
		s.afterAsideDir(aside)
	}
	if err := os.Rename(s.Path, moved); err != nil {
		_ = os.Remove(aside)
		return "", err
	}
	if s.afterMove != nil {
		s.afterMove()
	}
	data, readErr := os.ReadFile(moved)
	pid, ok := daemonMetadataPID(data, readErr)
	if !ok || pid != s.PID || s.alive(pid) {
		return moved, s.moveBack(moved, aside)
	}
	return moved, nil
}

// moveBack puts the moved file back at Path after the daemon started during recovery. It leaves the file
// in aside, and says where, when a new user.json already exists.
func (s StaleDaemon) moveBack(moved, aside string) error {
	if err := os.Link(moved, s.Path); err != nil {
		return fmt.Errorf("the mcporter daemon started during recovery and wrote new daemon metadata to %s; "+
			"the metadata moved aside is left in %s (not moved back: %v). Run 'ap mcp login' again", s.Path, aside, err)
	}
	if err := os.Remove(moved); err != nil {
		return err
	}
	_ = os.Remove(aside)
	return fmt.Errorf("the mcporter daemon started during recovery; its daemon metadata was moved back to %s. "+
		"Run 'ap mcp login' again", s.Path)
}

// daemonMetadataPID reads the pid from daemon metadata bytes. ok is false when the read failed or the pid is
// not a positive integer.
func daemonMetadataPID(data []byte, readErr error) (int, bool) {
	if readErr != nil {
		return 0, false
	}
	var md struct {
		PID int `json:"pid"`
	}
	if json.Unmarshal(data, &md) != nil || md.PID <= 0 {
		return 0, false
	}
	return md.PID, true
}

const (
	// daemonExitPoll and daemonExitPolls bound the wait for leftover processes after SIGTERM: 100 ms, 5 s.
	daemonExitPoll  = 100 * time.Millisecond
	daemonExitPolls = 50
)

// WaitExited polls PidRunning until none of pids runs, for up to 5 s. It errors and names the pids still
// running. A nil PidRunning counts every pid as running.
func (d Daemon) WaitExited(pids []int) error {
	for i := 0; ; i++ {
		var alive []int
		for _, pid := range pids {
			if d.PidRunning == nil || d.PidRunning(pid) {
				alive = append(alive, pid)
			}
		}
		if len(alive) == 0 {
			return nil
		}
		if i == daemonExitPolls {
			return fmt.Errorf("mcporter processes %v are still running 5s after SIGTERM; nothing was cleared or started", alive)
		}
		if d.Sleep != nil {
			d.Sleep(daemonExitPoll)
		}
	}
}

// refuseSymlink errors when path is a symlink. A missing path is fine.
func refuseSymlink(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("refusing to touch %s: it is a symlink", path)
	}
	return nil
}
