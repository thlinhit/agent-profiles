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
func (Adapter) InitTemplate() []byte { return []byte("{}\n") }
