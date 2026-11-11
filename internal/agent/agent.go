package agent

import (
	"fmt"
	"github.com/thlinhit/agent-profiles/internal/agent/claude"
	"github.com/thlinhit/agent-profiles/internal/agent/pi"
)

type Agent interface {
	Name() string
	EnvVar() string
	DefaultConfigDir() (string, error)
	InitTemplate() []byte
}

func All() []Agent { return []Agent{claude.Adapter{}, pi.Adapter{}} }
func Get(name string) (Agent, error) {
	for _, a := range All() {
		if a.Name() == name {
			return a, nil
		}
	}
	return nil, fmt.Errorf("unknown agent %q", name)
}
