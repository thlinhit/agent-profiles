package main

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"slices"

	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/config"
	"github.com/thlinhit/agent-profiles/internal/mcp"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

// Test hooks: tests replace these so they never read the real MCP pool or run mcporter.
var (
	mcpPoolPath  = config.MCPPoolPath
	mcpAvailable = mcp.SystemAvailable
)

// runMCP runs "ap mcp <subcommand>". Only sync exists for now.
func runMCP(s profile.Store, args []string, out io.Writer) error {
	if len(args) != 2 || args[1] != "sync" {
		return errors.New(commandUsage("mcp"))
	}
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
