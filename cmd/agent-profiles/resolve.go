package main

import (
	"github.com/thlinhit/agent-profiles/internal/agent"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

func resolveProfile(s profile.Store, agentName, name string) (agent.Agent, string, error) {
	a, err := agent.Get(agentName)
	if err != nil {
		return nil, "", err
	}
	dir, err := s.Resolve(a.Name(), name)
	if err != nil {
		return nil, "", err
	}
	return a, dir, nil
}
