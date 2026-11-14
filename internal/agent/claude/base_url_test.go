package claude_test

import (
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/agent/claude"
)

func TestBaseURLValidationIsFlagNeutral(t *testing.T) {
	for _, input := range []string{"http://%", "file:///etc/passwd", "http://", "http://user:token@host", "http://host?key=x", "http://host#fragment"} {
		t.Run(input, func(t *testing.T) {
			_, err := (claude.Adapter{}).WithBaseURL(nil, input)
			if err == nil || !strings.HasPrefix(err.Error(), "invalid base URL") || strings.Contains(err.Error(), "--base-url") {
				t.Fatalf("WithBaseURL(%q) = %v; want flag-neutral validation error", input, err)
			}
		})
	}
}
