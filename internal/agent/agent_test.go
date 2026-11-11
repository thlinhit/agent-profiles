package agent_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/linhtruong/agent-profiles/internal/agent"
)

func TestRegistryGet(t *testing.T) {
	for _, tc := range []struct {
		label, name string
		wantError   bool
	}{
		{"known", "claude", false},
		{"unknown", "unknown", true},
		{"empty", "", true},
		{"case_sensitive", "Claude", true},
	} {
		t.Run(tc.label, func(t *testing.T) {
			a, err := agent.Get(tc.name)
			if tc.wantError {
				if err == nil || !strings.Contains(err.Error(), "unknown agent") || a != nil {
					t.Fatalf("Get(%q) = %v, %v; want nil agent and unknown-agent error", tc.name, a, err)
				}
				return
			}
			if err != nil || a == nil {
				t.Fatalf("Get(%q) = %v, %v", tc.name, a, err)
			}
			if a.Name() != tc.name || a.EnvVar() != "CLAUDE_CONFIG_DIR" {
				t.Fatalf("unexpected adapter: name=%q env=%q", a.Name(), a.EnvVar())
			}
		})
	}
}

func TestRegistryAll(t *testing.T) {
	var names []string
	seen := map[string]bool{}
	for _, a := range agent.All() {
		if a == nil {
			t.Fatal("nil adapter in registry")
		}
		name := a.Name()
		if seen[name] {
			t.Fatalf("duplicate registered name %q", name)
		}
		seen[name] = true
		names = append(names, name)
		found, err := agent.Get(name)
		if err != nil || found == nil || found.Name() != name {
			t.Fatalf("listed adapter %q cannot be looked up: %v", name, err)
		}
	}
	if want := []string{"claude"}; !slices.Equal(names, want) {
		t.Fatalf("registry order = %q; want %q", names, want)
	}
}
