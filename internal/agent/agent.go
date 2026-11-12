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

// BaseURLSetter is implemented by agents whose settings can point at a
// custom API endpoint (for example a local proxy).
type BaseURLSetter interface {
	// WithBaseURL returns settings with the base URL applied. A nil
	// template means start from an empty settings object.
	WithBaseURL(template []byte, baseURL string) ([]byte, error)
}

// AuthTokenSetter is implemented by agents whose settings can carry an API
// auth token (for example for a local proxy).
type AuthTokenSetter interface {
	WithAuthToken(template []byte, token string) ([]byte, error)
}

// SettingsUpdater is implemented by agents that can refresh the tool-owned
// parts of an existing profile's settings (for example the status line)
// without touching what the user may have edited.
type SettingsUpdater interface {
	// UpdateSettings returns current with tool-owned keys refreshed. A nil
	// current means the profile has no settings file yet.
	UpdateSettings(current []byte) ([]byte, error)
}

func All()[]Agent { return []Agent{claude.Adapter{}, pi.Adapter{}} }
func Get(name string) (Agent, error) {
	for _, a := range All() {
		if a.Name() == name {
			return a, nil
		}
	}
	return nil, fmt.Errorf("unknown agent %q", name)
}
