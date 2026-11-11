package pi

import (
	"os"
	"path/filepath"
)

type Adapter struct{}

func (Adapter) Name() string   { return "pi" }
func (Adapter) EnvVar() string { return "PI_CODING_AGENT_DIR" }
func (Adapter) DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pi", "agent"), nil
}
func (Adapter) InitTemplate() []byte { return []byte("{}\n") }
