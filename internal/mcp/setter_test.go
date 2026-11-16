package mcp_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/mcp"
)

func TestOAuthEntryShape(t *testing.T) {
	got := mcp.OAuthEntry("atlassian", mcp.Proxy{ConfigPath: "/data/agent-profiles/mcporter.json"})
	data, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var want any
	if err := json.Unmarshal([]byte(`{"command":"mcporter","args":["serve","--stdio","--servers","atlassian"],"env":{"MCPORTER_CONFIG":"/data/agent-profiles/mcporter.json","MCPORTER_OAUTH_NO_BROWSER":"1"}}`), &want); err != nil {
		t.Fatal(err)
	}
	var have any
	if err := json.Unmarshal(data, &have); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(have, want) {
		t.Fatalf("OAuthEntry = %s", data)
	}
}
