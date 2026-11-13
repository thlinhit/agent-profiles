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
	SkillsSubdir() string
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

// SettingsUpdater is implemented by agents that can refresh the ap-owned
// parts of an existing profile's settings (for example the status line)
// without touching what the user may have edited.
type SettingsUpdater interface {
	// UpdateSettings returns current with ap-owned keys refreshed. A nil
	// current means the profile has no settings file yet.
	UpdateSettings(current []byte) ([]byte, error)
}

// SkillDirsSetter is implemented by agents that must be told about the real
// directories behind linked skills. Claude Code resolves skill symlinks and
// prompts before reading outside its working directories, so the targets are
// added as additional directories (Linked-skill access).
type SkillDirsSetter interface {
	// SetSkillDirs drops prev entries that are not in next, appends missing
	// next entries, and keeps everything else the user configured.
	SetSkillDirs(current []byte, prev, next []string) ([]byte, error)
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
