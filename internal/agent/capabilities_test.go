package agent_test

import (
	"testing"

	"github.com/thlinhit/agent-profiles/internal/agent"
)

func TestAgentCapabilities(t *testing.T) {
	for _, name := range []string{"claude", "pi"} {
		t.Run(name, func(t *testing.T) {
			a, err := agent.Get(name)
			if err != nil {
				t.Fatal(err)
			}
			if a.Binary() != name {
				t.Fatalf("Binary() = %q, want %q", a.Binary(), name)
			}
			hinter, hasHint := a.(agent.PluginSkillsHinter)
			setter, hasToken := a.(agent.AuthTokenSetter)
			if hasHint != (name == "claude") || hasToken != (name == "claude") {
				t.Fatalf("%s capabilities: hint=%v token=%v", name, hasHint, hasToken)
			}
			if hasHint && hinter.PluginSkillsHint() != "(plugin skills: manage with /plugin inside this profile)" {
				t.Fatalf("PluginSkillsHint() = %q", hinter.PluginSkillsHint())
			}
			if hasToken && setter.AuthTokenLabel() != "ANTHROPIC_AUTH_TOKEN" {
				t.Fatalf("AuthTokenLabel() = %q", setter.AuthTokenLabel())
			}
		})
	}
}
