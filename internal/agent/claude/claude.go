package claude

import (
	"encoding/json"
	"fmt"
	"net/url"
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

// WithBaseURL sets env.ANTHROPIC_BASE_URL in the settings template. Only
// http(s) URLs with a host and no embedded credentials, query or fragment
// are accepted, so tokens never end up in the URL.
func (Adapter) WithBaseURL(template []byte, baseURL string) ([]byte, error) {
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid --base-url: %w", err)
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("invalid --base-url %q: want http(s)://host[:port][/path]", baseURL)
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.ForceQuery {
		return nil, fmt.Errorf("invalid --base-url %q: credentials, query and fragment are not allowed", baseURL)
	}
	return setEnv(template, "ANTHROPIC_BASE_URL", baseURL)
}

// WithAuthToken sets env.ANTHROPIC_AUTH_TOKEN in the settings template.
func (Adapter) WithAuthToken(template []byte, token string) ([]byte, error) {
	if token == "" {
		return nil, fmt.Errorf("auth token is empty")
	}
	for _, r := range token {
		if r <= ' ' || r == 0x7f {
			return nil, fmt.Errorf("auth token must not contain whitespace or control characters")
		}
	}
	return setEnv(template, "ANTHROPIC_AUTH_TOKEN", token)
}

func setEnv(template []byte, key, value string) ([]byte, error) {
	settings := map[string]any{}
	if len(template) > 0 {
		if err := json.Unmarshal(template, &settings); err != nil {
			return nil, fmt.Errorf("parse settings template: %w", err)
		}
	}
	env, _ := settings["env"].(map[string]any)
	if env == nil {
		env = map[string]any{}
	}
	env[key] = value
	settings["env"] = env
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
