package claude

import (
	"os"
	"path/filepath"
)

type Adapter struct{}

func (Adapter) Name() string   { return "claude" }
func (Adapter) EnvVar() string { return "CLAUDE_CONFIG_DIR" }
func (Adapter) DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

const initTemplate = `{
  "permissions": {
    "allow": ["Edit", "Bash(git *)"],
    "ask": ["Bash(rm *)", "Bash(git push *)", "Bash(curl *)", "Bash(git -c *)", "Bash(git config *)", "Bash(git -C *)", "Bash(git --*)", "Bash(git submodule *)", "Bash(git difftool *)", "WebFetch"],
    "defaultMode": "auto"
  }
}
`

func (Adapter) InitTemplate() []byte { return []byte(initTemplate) }
