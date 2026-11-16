package mcp_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/mcp"
)

func TestMergeServersKeepsOtherKeysAndDropsPrev(t *testing.T) {
	cur := `{"numStartups":4,"mcpServers":{"gone":{"command":"bye"},"hand":{"command":"mine"}}}`
	out, err := mcp.MergeServers([]byte(cur), []string{"gone"}, map[string]json.RawMessage{"new": json.RawMessage(`{"command":"n"}`)})
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"mcpServers\": {\n    \"hand\": {\n      \"command\": \"mine\"\n    },\n    \"new\": {\n      \"command\": \"n\"\n    }\n  },\n  \"numStartups\": 4\n}\n"
	if string(out) != want {
		t.Fatalf("MergeServers = %q\nwant %q", out, want)
	}
}

func TestMergeServersMergedMCPServersMustBeObject(t *testing.T) {
	if _, err := mcp.MergeServers([]byte(`{"mcpServers":[]}`), nil, nil); err == nil || !strings.Contains(err.Error(), "mcpServers is not a JSON object") {
		t.Fatalf("err = %v", err)
	}
}
