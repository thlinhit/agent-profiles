package main

import (
	"encoding/json"
	"os"
	"slices"
	"testing"
)

// TestManagedSettingsMCPDenyRules pins the Managed deny rules that keep Claude away from mcporter,
// the Shared login's credentials file and its proxy command. The file is installed by hand, so this
// test is the only check that the repository copy keeps them.
func TestManagedSettingsMCPDenyRules(t *testing.T) {
	data, err := os.ReadFile("../../managed/claude/managed-settings.json")
	if err != nil {
		t.Fatal(err)
	}
	var settings struct {
		Permissions struct {
			Deny []string `json:"deny"`
		} `json:"permissions"`
	}
	if err := json.Unmarshal(data, &settings); err != nil {
		t.Fatalf("managed-settings.json is not valid JSON: %v", err)
	}
	for _, rule := range []string{
		"Bash(mcporter *)",
		"Read(**/agent-profiles/mcporter*)",
		"Read(**/mcporter/credentials.json)",
		"Read(**/.mcporter/**)",
	} {
		if !slices.Contains(settings.Permissions.Deny, rule) {
			t.Errorf("permissions.deny is missing %q", rule)
		}
	}
}
