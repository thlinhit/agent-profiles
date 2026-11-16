package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"regexp"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/config"
	"github.com/thlinhit/agent-profiles/internal/mcp"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// Test hooks: tests replace these so they never read the real MCP pool or run mcporter.
var (
	mcpPoolPath  = config.MCPPoolPath
	mcpAvailable = mcp.SystemAvailable
	// mcporterAuth runs "mcporter auth <name>" in the terminal with MCPORTER_CONFIG set to configPath.
	mcporterAuth = runMcporterAuth
)

const (
	mcpAddUsage    = "mcp add-server <name> (--url <url> | -- <cmd> [args...]) [--auth oauth] [--header K=V]... [--env K=V]..."
	mcpRemoveUsage = "mcp remove-server <name>"
	mcpLoginUsage  = "mcp login <name>"
)

// runMCP runs "ap mcp <subcommand>".
func runMCP(s profile.Store, args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New(commandUsage("mcp"))
	}
	switch args[1] {
	case "sync":
		if len(args) != 2 {
			return mcpUsageError("mcp sync", "mcp sync", "")
		}
		return runMCPSync(s, out)
	case "add-server":
		return runMCPAddServer(args[2:], out)
	case "remove-server":
		return runMCPRemoveServer(args[2:], out)
	case "list":
		if len(args) != 2 {
			return mcpUsageError("mcp list", "mcp list", "")
		}
		return runMCPList(out)
	case "login":
		return runMCPLogin(args[2:])
	}
	return errors.New(commandUsage("mcp"))
}

// runMCPList prints one row per Pool server: name, kind and, for OAuth servers, login status.
func runMCPList(out io.Writer) error {
	path, err := mcpPoolPath()
	if err != nil {
		return err
	}
	pool, _, err := mcp.Load(path)
	if err != nil {
		return err
	}
	if len(pool.Servers) == 0 {
		_, err := fmt.Fprintln(out, "no servers in the MCP pool")
		return err
	}
	names := slices.Sorted(maps.Keys(pool.Servers))
	var reason, credsPath string
	var credsErr error
	for _, name := range names {
		if pool.Servers[name].Auth == "oauth" {
			reason = mcpAvailable()
			credsPath, credsErr = mcp.CredentialsPath()
			break
		}
	}
	w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
	firstLoggedOut := ""
	for _, name := range names {
		if pool.Servers[name].Auth != "oauth" {
			if _, err := fmt.Fprintf(w, "%s\tdirect\n", name); err != nil {
				return err
			}
			continue
		}
		status := mcpOAuthStatus(reason, credsPath, credsErr, name)
		if status == "not logged in" && firstLoggedOut == "" {
			firstLoggedOut = name
		}
		if _, err := fmt.Fprintf(w, "%s\toauth\t%s\n", name, status); err != nil {
			return err
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	if firstLoggedOut != "" {
		_, err = fmt.Fprintf(out, "run 'ap mcp login %s'\n", firstLoggedOut)
		return err
	}
	return nil
}

// mcpOAuthStatus words the Shared login state of one OAuth Pool server.
// unavailable is the reason mcporter cannot be used, or "" when it can.
func mcpOAuthStatus(unavailable, credsPath string, credsErr error, name string) string {
	if unavailable != "" {
		return "unavailable"
	}
	if credsErr != nil {
		return "unknown (cannot locate credentials file)"
	}
	st := mcp.ReadLoginStatus(credsPath, name)
	switch {
	case st.Unknown != "":
		return "unknown (" + st.Unknown + ")"
	case !st.LoggedIn:
		return "not logged in"
	case st.Expires.IsZero():
		return "logged in"
	default:
		return "logged in (expires " + st.Expires.Local().Format("2006-01-02 15:04") + ")"
	}
}

// runMCPLogin runs mcporter's browser login for one OAuth Pool server, so the tokens land in the Shared login.
// It regenerates ap's mcporter config first, and refuses servers that are not OAuth.
func runMCPLogin(args []string) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return mcpUsageError(mcpLoginUsage, "mcp login", "")
	}
	name := args[0]
	path, err := mcpPoolPath()
	if err != nil {
		return err
	}
	pool, _, err := mcp.Load(path)
	if err != nil {
		return err
	}
	server, ok := pool.Servers[name]
	if !ok {
		return fmt.Errorf("MCP pool has no server %q", name)
	}
	if server.Auth != "oauth" {
		return fmt.Errorf("%s is not an OAuth server", name)
	}
	if reason := mcpAvailable(); reason != "" {
		return errors.New(reason)
	}
	configPath, err := mcp.McporterConfigPath()
	if err != nil {
		return err
	}
	if _, err := mcp.WriteMcporterConfig(configPath, pool); err != nil {
		return err
	}
	return mcporterAuth(name, configPath)
}

// runMcporterAuth runs "mcporter auth <name>" with the process's terminal attached.
func runMcporterAuth(name, configPath string) error {
	cmd := exec.Command("mcporter", "auth", name)
	cmd.Env = append(os.Environ(), "MCPORTER_CONFIG="+configPath)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// mcpUsageError builds a usage error in the same shape as commandUsage. reason, if set, comes first.
func mcpUsageError(usage, help, reason string) error {
	msg := "usage: ap " + usage + "\nRun 'ap help " + help + "' for details."
	if reason != "" {
		msg = reason + "\n" + msg
	}
	return errors.New(msg)
}

// runMCPSync writes ap's mcporter config, then syncs every Profile of every MCP-capable Agent.
func runMCPSync(s profile.Store, out io.Writer) error {
	pool, proxy, changed, err := loadMCP()
	if err != nil {
		return err
	}
	if changed {
		if _, err := fmt.Fprintln(out, "mcporter: updated"); err != nil {
			return err
		}
	}
	for _, a := range mcpAgents() {
		names, err := s.List(a.Name())
		if err != nil {
			return err
		}
		for _, name := range names {
			dir, err := s.Resolve(a.Name(), name)
			if err != nil {
				return err
			}
			label := a.Name() + "/" + name
			n, err := syncMCPProfile(a, dir, label, pool, proxy, out)
			if err != nil {
				return err
			}
			if n == 0 {
				if _, err := fmt.Fprintf(out, "%s: unchanged\n", label); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// runMCPAddServer adds one Pool server. It touches only the MCP pool file, never a Profile or mcporter.
func runMCPAddServer(args []string, out io.Writer) error {
	name, server, err := parseAddServer(args)
	if err != nil {
		return err
	}
	if err := mcp.Validate(name, server); err != nil {
		return err
	}
	path, err := mcpPoolPath()
	if err != nil {
		return err
	}
	pool, _, err := mcp.Load(path)
	if err != nil {
		return err
	}
	if _, dup := pool.Servers[name]; dup {
		return fmt.Errorf("MCP pool already has %q; run 'ap mcp remove-server %s' first", name, name)
	}
	if err := warnLiteralValues(out, name, server); err != nil {
		return err
	}
	pool.Servers[name] = server
	if err := mcp.Save(path, pool); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "pool: + %s\n", name)
	return err
}

// mcpReference matches a ${...} reference, which is how secrets should be written.
var mcpReference = regexp.MustCompile(`\$\{[^}]+\}`)

// warnLiteralValues prints one warning per --header or --env value that has no ${...} reference.
// The warning names the key only; the value is never printed. The server is still saved.
func warnLiteralValues(out io.Writer, name string, s mcp.Server) error {
	warn := func(kind string, m map[string]string) error {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		for _, k := range keys {
			if mcpReference.MatchString(m[k]) {
				continue
			}
			if _, err := fmt.Fprintf(out, "! %s: %s %s holds a literal value; use ${VAR} instead\n", name, kind, k); err != nil {
				return err
			}
		}
		return nil
	}
	if err := warn("header", s.Headers); err != nil {
		return err
	}
	return warn("env", s.Env)
}

// runMCPRemoveServer removes one Pool server. Profiles change only on the next "ap mcp sync".
func runMCPRemoveServer(args []string, out io.Writer) error {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		return mcpUsageError(mcpRemoveUsage, "mcp remove-server", "")
	}
	name := args[0]
	path, err := mcpPoolPath()
	if err != nil {
		return err
	}
	pool, _, err := mcp.Load(path)
	if err != nil {
		return err
	}
	if _, ok := pool.Servers[name]; !ok {
		return fmt.Errorf("MCP pool has no server %q", name)
	}
	delete(pool.Servers, name)
	if err := mcp.Save(path, pool); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(out, "pool: - %s\n", name); err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, "run 'ap mcp sync' to update Profiles")
	return err
}

// parseAddServer reads "<name> (--url U | -- cmd args...) [flags]" into a Pool server.
// Every error it returns is a usage error, so nothing is read or written.
// Flags must come before "--"; everything after "--" is the stdio command.
func parseAddServer(args []string) (string, mcp.Server, error) {
	usage := func(reason string) error { return mcpUsageError(mcpAddUsage, "mcp add-server", reason) }
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", mcp.Server{}, usage("")
	}
	name := args[0]
	var (
		url, auth    string
		cmd          []string
		headers, env = map[string]string{}, map[string]string{}
		sawDash      bool
		seen         = map[string]bool{}
	)
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		if arg == "--" {
			sawDash = true
			cmd = rest[i+1:]
			break
		}
		switch arg {
		case "--url", "--auth", "--header", "--env":
		default:
			return "", mcp.Server{}, usage(fmt.Sprintf("unexpected argument %q", arg))
		}
		if (arg == "--url" || arg == "--auth") && seen[arg] {
			return "", mcp.Server{}, usage(arg + " given twice")
		}
		seen[arg] = true
		if i+1 >= len(rest) {
			return "", mcp.Server{}, usage(arg + " needs a value")
		}
		i++
		val := rest[i]
		switch arg {
		case "--url":
			url = val
		case "--auth":
			auth = val
		case "--header", "--env":
			k, v, ok := splitKV(val)
			if !ok {
				return "", mcp.Server{}, usage(fmt.Sprintf("%s must be KEY=VALUE, got %q", arg, val))
			}
			m := headers
			if arg == "--env" {
				m = env
			}
			if _, dup := m[k]; dup {
				return "", mcp.Server{}, usage(fmt.Sprintf("%s %s given twice", arg, k))
			}
			m[k] = v
		}
	}
	haveURL := seen["--url"]
	haveAuth := seen["--auth"]
	switch {
	case haveURL && sawDash:
		return "", mcp.Server{}, usage("use either --url or -- <cmd>, not both")
	case !haveURL && !sawDash:
		return "", mcp.Server{}, usage("set --url <url> or -- <cmd> [args...]")
	case sawDash && len(cmd) == 0:
		return "", mcp.Server{}, usage("-- needs a command")
	case haveAuth && auth != "oauth":
		return "", mcp.Server{}, usage(fmt.Sprintf("--auth must be \"oauth\", got %q", auth))
	case haveAuth && !haveURL:
		return "", mcp.Server{}, usage("--auth needs --url")
	case len(headers) > 0 && !haveURL:
		return "", mcp.Server{}, usage("--header needs --url")
	case len(env) > 0 && !sawDash:
		return "", mcp.Server{}, usage("--env needs a stdio server (-- <cmd>)")
	}
	if haveURL {
		return name, mcp.Server{Type: "http", URL: url, Auth: auth, Headers: nilIfEmpty(headers)}, nil
	}
	server := mcp.Server{Type: "stdio", Command: cmd[0], Env: nilIfEmpty(env)}
	if len(cmd) > 1 {
		server.Args = cmd[1:]
	}
	return name, server, nil
}

// splitKV splits KEY=VALUE. The key must be non-empty; the value may be empty.
func splitKV(s string) (string, string, bool) {
	k, v, ok := strings.Cut(s, "=")
	return k, v, ok && k != ""
}

func nilIfEmpty(m map[string]string) map[string]string {
	if len(m) == 0 {
		return nil
	}
	return m
}

// mcpSubcommand returns the help entry for "ap help mcp <sub>", or nil for an unknown sub.
func mcpSubcommand(sub string) *command {
	switch sub {
	case "sync":
		return &command{name: "mcp", usage: "mcp sync", summary: "Sync the MCP pool into Profiles", examples: []string{"ap mcp sync"}}
	case "add-server":
		return &command{
			name:      "mcp",
			usage:     mcpAddUsage,
			summary:   "Add a server to the MCP pool",
			arguments: []string{"<name>\tServer name: lowercase letters, digits, '-' or '_'", "-- <cmd> [args...]\tStdio command, after --"},
			flags:     []string{"--url <url>\tHTTP server URL", "--auth oauth\tShared login through mcporter (needs --url)", "--header K=V\tHTTP header, repeatable (needs --url)", "--env K=V\tEnvironment variable, repeatable (needs -- <cmd>)"},
			examples:  []string{"ap mcp add-server atlassian --url https://mcp.atlassian.com/v1/mcp --auth oauth", "ap mcp add-server docs --env LOG=debug -- npx -y docs-mcp"},
			note:      "Writes the MCP pool only; run 'ap mcp sync' to update Profiles. Values are stored as typed: ${VAR} is never expanded here.",
		}
	case "list":
		return &command{name: "mcp", usage: "mcp list", summary: "List Pool servers and their login status", examples: []string{"ap mcp list"}, note: "OAuth status comes from the Shared login's metadata only; no token is read or printed."}
	case "login":
		return &command{
			name:      "mcp",
			usage:     mcpLoginUsage,
			summary:   "Log in an OAuth Pool server (Shared login)",
			arguments: []string{"<name>\tOAuth Pool server name"},
			examples:  []string{"ap mcp login atlassian"},
			note:      "Runs mcporter's browser login in this terminal. The tokens are kept outside every Profile.",
		}
	case "remove-server":
		return &command{
			name:      "mcp",
			usage:     mcpRemoveUsage,
			summary:   "Remove a server from the MCP pool",
			arguments: []string{"<name>\tPool server name"},
			examples:  []string{"ap mcp remove-server atlassian"},
			note:      "Writes the MCP pool only; run 'ap mcp sync' to remove the Managed servers from Profiles.",
		}
	}
	return nil
}

// loadMCP reads the MCP pool and the proxy for OAuth Pool servers. A missing pool is empty.
// When mcporter is available, ap's mcporter config is written first; changed reports whether its bytes changed.
// When it is not, the config is left alone and the proxy carries the reason.
func loadMCP() (pool mcp.Pool, proxy mcp.Proxy, changed bool, err error) {
	poolPath, err := mcpPoolPath()
	if err != nil {
		return mcp.Pool{}, mcp.Proxy{}, false, err
	}
	pool, _, err = mcp.Load(poolPath)
	if err != nil {
		return mcp.Pool{}, mcp.Proxy{}, false, err
	}
	configPath, err := mcp.McporterConfigPath()
	if err != nil {
		return mcp.Pool{}, mcp.Proxy{}, false, err
	}
	proxy = mcp.Proxy{ConfigPath: configPath, Unavailable: mcpAvailable()}
	if proxy.Unavailable != "" {
		return pool, proxy, false, nil
	}
	changed, err = mcp.WriteMcporterConfig(configPath, pool)
	if err != nil {
		return mcp.Pool{}, mcp.Proxy{}, false, err
	}
	return pool, proxy, changed, nil
}

// mcpAgents returns the Agents that implement agent.MCPSetter, sorted by name.
func mcpAgents() []agent.Agent {
	var agents []agent.Agent
	for _, a := range agent.All() {
		if _, ok := a.(agent.MCPSetter); ok {
			agents = append(agents, a)
		}
	}
	slices.SortFunc(agents, func(x, y agent.Agent) int { return cmp.Compare(x.Name(), y.Name()) })
	return agents
}

// syncMCPProfile syncs one Profile dir of a with pool. It prints one "label: line" per report line
// and returns how many it printed, so the caller can print "unchanged" when that is zero.
func syncMCPProfile(a agent.Agent, dir, label string, pool mcp.Pool, proxy mcp.Proxy, out io.Writer) (int, error) {
	setter, ok := a.(agent.MCPSetter)
	if !ok {
		return 0, fmt.Errorf("%s: agent has no MCP support", label)
	}
	report, err := mcp.Sync(setter, dir, pool, proxy)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", label, err)
	}
	for _, line := range report.Lines {
		if _, err := fmt.Fprintf(out, "%s: %s\n", label, line); err != nil {
			return 0, err
		}
	}
	return len(report.Lines), nil
}
