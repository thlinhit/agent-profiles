package claude

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

type Adapter struct{}

func (Adapter) SkillsSubdir() string { return "skills" }

func (Adapter) Name() string   { return "claude" }
func (Adapter) EnvVar() string { return "CLAUDE_CONFIG_DIR" }
func (Adapter) DefaultConfigDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude"), nil
}

// statusLineCommand renders "profile │ model │ 🧠 ctx% │ dir (branch*)" from
// the JSON Claude Code pipes on stdin. The profile gets an emoji chosen by
// a cksum of its name, so each profile keeps the same one. Colors follow cc-statusline: profile
// magenta, model blue, context green/orange/yellow/red by usage, dir cyan,
// branch green with a red dirty marker, dim separators. Missing data drops
// its segment. Needs jq; git is optional.
const statusLineCommand = `input=$(cat)
c() { printf '\033[%sm' "$1"; }
r=$(c 0)
sep=" $(c 2)│$r "
line=
add() { if [ -n "$1" ]; then line="${line:+$line$sep}$1"; fi; }
if [ -n "$CLAUDE_CONFIG_DIR" ]; then
  name=$(basename "$CLAUDE_CONFIG_DIR")
  set -- 🦊 🐙 🦉 🐝 🐢 🦄 🐳 🦖 🐧 🦜 🐼 🦁
  shift $(( $(printf '%s' "$name" | cksum | cut -d' ' -f1) % $# ))
  add "$1 $(c '38;2;180;140;255')$name$r"
fi
model=$(printf '%s' "$input" | jq -r '.model.display_name // empty' 2>/dev/null)
if [ -n "$model" ]; then add "$(c '38;2;0;153;255')$model$r"; fi
pct=$(printf '%s' "$input" | jq -r '.context_window | select(.current_usage != null) | (.context_window_size // 200000) as $s | (if $s > 0 then $s else 200000 end) as $w | .current_usage | ((.input_tokens // 0) + (.cache_creation_input_tokens // 0) + (.cache_read_input_tokens // 0)) * 100 / $w | floor' 2>/dev/null)
if [ -n "$pct" ]; then
  if [ "$pct" -ge 90 ]; then pc='38;2;255;85;85'
  elif [ "$pct" -ge 70 ]; then pc='38;2;230;200;0'
  elif [ "$pct" -ge 50 ]; then pc='38;2;255;176;85'
  else pc='38;2;0;175;80'; fi
  add "🧠 $(c "$pc")$pct%$r"
fi
cwd=$(printf '%s' "$input" | jq -r '.cwd // empty' 2>/dev/null)
if [ -n "$cwd" ]; then
  dir="$(c '38;2;86;182;194')$(basename "$cwd")$r"
  branch=$(git -C "$cwd" symbolic-ref --short HEAD 2>/dev/null)
  if [ -n "$branch" ]; then
    dirty=
    if [ -n "$(git -C "$cwd" --no-optional-locks status --porcelain 2>/dev/null)" ]; then dirty='*'; fi
    g=$(c '38;2;0;175;80')
    dir="$dir $g($branch$(c '38;2;255;85;85')$dirty$g)$r"
  fi
  add "$dir"
fi
printf '%s' "$line"`

const permissionsTemplate = `{
  "allow": ["Edit", "Bash(git *)"],
  "ask": ["Bash(rm *)", "Bash(git push *)", "Bash(curl *)", "Bash(git -c *)", "Bash(git config *)", "Bash(git -C *)", "Bash(git --*)", "Bash(git submodule *)", "Bash(git difftool *)", "WebFetch"],
  "defaultMode": "auto"
}`

// initTemplate is built once so the status line script can stay readable
// above instead of living as an escaped JSON string.
var initTemplate = func() string {
	statusLine, err := json.Marshal(map[string]string{"type": "command", "command": statusLineCommand})
	if err != nil {
		panic(err)
	}
	out, err := json.MarshalIndent(map[string]json.RawMessage{
		"permissions": json.RawMessage(permissionsTemplate),
		"statusLine":  statusLine,
	}, "", "  ")
	if err != nil {
		panic(err)
	}
	return string(out) + "\n"
}()

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

// UpdateSettings replaces statusLine with the one from the init template.
// permissions and env are seeded once and belong to the user afterwards.
func (Adapter) UpdateSettings(current []byte) ([]byte, error) {
	var tmpl map[string]json.RawMessage
	if err := json.Unmarshal([]byte(initTemplate), &tmpl); err != nil {
		return nil, err
	}
	settings := map[string]json.RawMessage{}
	if len(current) > 0 {
		if err := json.Unmarshal(current, &settings); err != nil {
			return nil, fmt.Errorf("parse settings: %w", err)
		}
		if settings == nil {
			return nil, fmt.Errorf("parse settings: not a JSON object")
		}
	}
	settings["statusLine"] = tmpl["statusLine"]
	out, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
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
