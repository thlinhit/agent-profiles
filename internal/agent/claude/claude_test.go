package claude_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/agent/claude"
)

func TestClaudeIdentity(t *testing.T) {
	a := claude.Adapter{}
	for _, tc := range []struct {
		name, got, want string
	}{
		{"Name", a.Name(), "claude"},
		{"EnvVar", a.EnvVar(), "CLAUDE_CONFIG_DIR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got != tc.want {
				t.Fatalf("%s() = %q; want %q", tc.name, tc.got, tc.want)
			}
		})
	}
}

func TestClaudeDefaultConfigDir(t *testing.T) {
	for _, state := range []string{"set", "empty", "unset"} {
		t.Run(state, func(t *testing.T) {
			home := t.TempDir()
			// Setenv registers restoration even when the test then unsets HOME.
			t.Setenv("HOME", home)
			switch state {
			case "empty":
				t.Setenv("HOME", "")
			case "unset":
				if err := os.Unsetenv("HOME"); err != nil {
					t.Fatal(err)
				}
			}
			dir, err := (claude.Adapter{}).DefaultConfigDir()
			if state != "set" {
				if err == nil || dir != "" {
					t.Fatalf("DefaultConfigDir() with %s HOME = %q, %v; want empty path and error", state, dir, err)
				}
				return
			}
			if want := filepath.Join(home, ".claude"); err != nil || dir != want {
				t.Fatalf("DefaultConfigDir() = %q, %v; want %q", dir, err, want)
			}
		})
	}
}

func TestClaudeInitTemplate(t *testing.T) {
	template := (claude.Adapter{}).InitTemplate()
	if !json.Valid(template) {
		t.Fatalf("InitTemplate() is not valid JSON: %q", template)
	}
	var settings struct {
		Permissions map[string]json.RawMessage `json:"permissions"`
	}
	if err := json.Unmarshal(template, &settings); err != nil {
		t.Fatal(err)
	}
	if _, ok := settings.Permissions["deny"]; ok {
		t.Fatal("InitTemplate() must not contain permissions.deny")
	}
	for key, want := range map[string]any{
		"allow":       []any{"Edit", "Bash(git *)"},
		"ask":         []any{"Bash(rm *)", "Bash(git push *)", "Bash(curl *)", "Bash(git -c *)", "Bash(git config *)", "Bash(git -C *)", "Bash(git --*)", "Bash(git submodule *)", "Bash(git difftool *)", "WebFetch"},
		"defaultMode": "auto",
	} {
		var got any
		if err := json.Unmarshal(settings.Permissions[key], &got); err != nil {
			t.Fatalf("permissions.%s: %v", key, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("permissions.%s = %v; want %v", key, got, want)
		}
	}
}

func TestClaudeInitTemplateStatusLine(t *testing.T) {
	var settings struct {
		StatusLine struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal((claude.Adapter{}).InitTemplate(), &settings); err != nil {
		t.Fatal(err)
	}
	if settings.StatusLine.Type != "command" {
		t.Fatalf("statusLine.type = %q; want command", settings.StatusLine.Type)
	}
	for _, tool := range []string{"jq", "git"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("status line tests require %s: %v", tool, err)
		}
	}
	repo := filepath.Join(t.TempDir(), "agent-profiles")
	if out, err := exec.Command("git", "init", "-b", "main", repo).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	nonGit := filepath.Join(t.TempDir(), "plain directory")
	if err := os.Mkdir(nonGit, 0700); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, cwd, fields, profile, want string
		dirty                            bool
	}{
		{"clean git", repo, `"model":{"display_name":"Opus 5.5"},"context_window":{"context_window_size":200000,"current_usage":{"input_tokens":10000,"cache_creation_input_tokens":15000,"cache_read_input_tokens":25000,"output_tokens":90000}}`, "work", "🐼 work │ Opus 5.5 │ 🧠 25% │ agent-profiles (main)", false},
		{"dirty git", repo, `"model":{"display_name":"Opus 5.5"},"context_window":{"context_window_size":200000,"current_usage":{"input_tokens":50000}}`, "work", "🐼 work │ Opus 5.5 │ 🧠 25% │ agent-profiles (main*)", true},
		{"non git", nonGit, `"model":{"display_name":"Opus 5.5"},"context_window":{"context_window_size":100000,"current_usage":{"input_tokens":25999}}`, "work", "🐼 work │ Opus 5.5 │ 🧠 25% │ plain directory", false},
		{"missing fields", "", ``, "work", "🐼 work", false},
		{"null fields", "", `"model":null,"context_window":null`, "work", "🐼 work", false},
		{"no current usage", "", `"model":{"display_name":"Sonnet"},"context_window":{"context_window_size":200000}`, "work", "🐼 work │ Sonnet", false},
		{"default window", "", `"context_window":{"current_usage":{"input_tokens":50000}}`, "work", "🐼 work │ 🧠 25%", false},
		{"zero window", "", `"context_window":{"context_window_size":0,"current_usage":{"input_tokens":50000}}`, "work", "🐼 work │ 🧠 25%", false},
		{"zero usage", "", `"context_window":{"current_usage":{}}`, "work", "🐼 work │ 🧠 0%", false},
		{"missing profile", nonGit, `"model":{"display_name":"Opus 5.5"}`, "", "Opus 5.5 │ plain directory", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.dirty {
				file := filepath.Join(repo, "untracked")
				if err := os.WriteFile(file, []byte("dirty"), 0600); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = os.Remove(file) })
			}
			fields := tc.fields
			if tc.cwd != "" {
				cwd, _ := json.Marshal(tc.cwd)
				if fields != "" {
					fields += ","
				}
				fields += `"cwd":` + string(cwd)
			}
			cmd := exec.Command("sh", "-c", settings.StatusLine.Command)
			cmd.Dir = nonGit // Git metadata must come from stdin's cwd, not the shell cwd.
			profile := ""
			if tc.profile != "" {
				profile = filepath.Join(t.TempDir(), "profiles with spaces", tc.profile)
			}
			cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+profile)
			cmd.Stdin = strings.NewReader("{" + fields + "}")
			var stderr strings.Builder
			cmd.Stderr = &stderr
			out, err := cmd.Output()
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("status line: %v; stderr = %q", err, stderr.String())
			}
			if got := ansi.ReplaceAllString(string(out), ""); got != tc.want {
				t.Fatalf("status line text = %q; want %q", got, tc.want)
			}
		})
	}
}

var ansi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestClaudeStatusLineProfileEmoji(t *testing.T) {
	var settings struct {
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal((claude.Adapter{}).InitTemplate(), &settings); err != nil {
		t.Fatal(err)
	}
	emoji := func(t *testing.T, profile string) string {
		t.Helper()
		cmd := exec.Command("sh", "-c", settings.StatusLine.Command)
		cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR=/x/"+profile)
		cmd.Stdin = strings.NewReader("{}")
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		e, name, ok := strings.Cut(ansi.ReplaceAllString(string(out), ""), " ")
		if !ok || name != profile {
			t.Fatalf("output = %q; want \"<emoji> %s\"", out, profile)
		}
		return e
	}
	set := map[string]bool{}
	for _, e := range strings.Fields("🦊 🐙 🦉 🐝 🐢 🦄 🐳 🦖 🐧 🦜 🐼 🦁") {
		set[e] = true
	}
	for profile, want := range map[string]string{"work": "🐼", "ai-hero": "🦉"} {
		if got := emoji(t, profile); got != want {
			t.Errorf("%s emoji = %q; want %q", profile, got, want)
		}
	}
	for _, profile := range []string{"a", "personal", "default", "x_y-9"} {
		got := emoji(t, profile)
		if !set[got] {
			t.Errorf("%s emoji %q not from the list", profile, got)
		}
		if again := emoji(t, profile); again != got {
			t.Errorf("%s emoji changed between runs: %q then %q", profile, got, again)
		}
	}
}

func TestClaudeStatusLineColors(t *testing.T) {
	var settings struct {
		StatusLine struct {
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal((claude.Adapter{}).InitTemplate(), &settings); err != nil {
		t.Fatal(err)
	}
	const (
		reset   = "\x1b[0m"
		dim     = "\x1b[2m"
		magenta = "\x1b[38;2;180;140;255m"
		blue    = "\x1b[38;2;0;153;255m"
		cyan    = "\x1b[38;2;86;182;194m"
		green   = "\x1b[38;2;0;175;80m"
		orange  = "\x1b[38;2;255;176;85m"
		yellow  = "\x1b[38;2;230;200;0m"
		red     = "\x1b[38;2;255;85;85m"
	)
	render := func(t *testing.T, input string) string {
		t.Helper()
		cmd := exec.Command("sh", "-c", settings.StatusLine.Command)
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR=/x/work")
		cmd.Stdin = strings.NewReader(input)
		out, err := cmd.Output()
		if err != nil {
			t.Fatal(err)
		}
		return string(out)
	}
	got := render(t, `{"model":{"display_name":"Opus 5.5"},"context_window":{"context_window_size":100,"current_usage":{"input_tokens":10}},"cwd":"/tmp/plain"}`)
	sep := " " + dim + "│" + reset + " "
	want := "🐼 " + magenta + "work" + reset + sep + blue + "Opus 5.5" + reset + sep + "🧠 " + green + "10%" + reset + sep + cyan + "plain" + reset
	if got != want {
		t.Fatalf("colored output = %q; want %q", got, want)
	}
	for _, tc := range []struct {
		tokens int
		color  string
	}{{49, green}, {50, orange}, {70, yellow}, {90, red}} {
		out := render(t, fmt.Sprintf(`{"context_window":{"context_window_size":100,"current_usage":{"input_tokens":%d}}}`, tc.tokens))
		if !strings.Contains(out, tc.color+fmt.Sprintf("%d%%", tc.tokens)) {
			t.Errorf("%d%% rendered %q; want color %q", tc.tokens, out, tc.color)
		}
	}
}
