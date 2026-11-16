package agent

import (
	"encoding/json"
	"fmt"

	"github.com/thlinhit/agent-profiles/internal/agent/claude"
	"github.com/thlinhit/agent-profiles/internal/agent/pi"
	"github.com/thlinhit/agent-profiles/internal/mcp"
	"github.com/thlinhit/agent-profiles/internal/profile"
)

type Agent interface {
	Name() string
	Binary() string
	EnvVar() string
	DefaultConfigDir() (string, error)
	InitTemplate() []byte
	SkillsSubdir() string
}

// PluginSkillsHinter is implemented by Agents with their own Plugin skill management.
type PluginSkillsHinter interface {
	PluginSkillsHint() string
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
	AuthTokenLabel() string
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

// MCPSetter is implemented by Agents that can load MCP servers from their config dir.
type MCPSetter interface {
	// MCPConfigFile returns the path of the Agent's MCP config inside dir.
	MCPConfigFile(dir string) string
	// Translate returns the Agent's entry for a Pool server, or a skip reason.
	Translate(name string, s mcp.Server, proxy mcp.Proxy) (entry json.RawMessage, skip string, err error)
	// SetServers writes next, drops prev Managed servers not in next, keeps everything else.
	SetServers(current []byte, prev []string, next map[string]json.RawMessage) ([]byte, error)
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

// GlobalInstructionsSetter is implemented by agents that can stop loading
// instruction files from outside the Profile (Global instructions). The state
// lives in the Profile's settings only; there is no ledger.
type GlobalInstructionsSetter interface {
	// GlobalInstructionsState reports whether settings exclude the instruction
	// files under home. A nil settings means the Profile has no settings file.
	GlobalInstructionsState(settings []byte, home string) (profile.SettingState, error)
	// WithGlobalInstructions returns settings with those files excluded (on is
	// false) or allowed (on is true). A nil result means no file is needed.
	WithGlobalInstructions(settings []byte, home string, on bool) ([]byte, error)
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
