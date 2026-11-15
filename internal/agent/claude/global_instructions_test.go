package claude

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

const testHome = "/home/tester"

func allExcludes(home string) []string {
	paths, _ := excludePaths(home)
	return paths
}

func settingsJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func excludesOf(t *testing.T, data []byte) ([]string, bool) {
	t.Helper()
	var m map[string]json.RawMessage
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("settings not JSON: %v\n%s", err, data)
	}
	raw, ok := m[excludesKey]
	if !ok {
		return nil, false
	}
	var list []string
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	return list, true
}

func TestExcludePathsAreAbsoluteAndOrdered(t *testing.T) {
	got, err := excludePaths(testHome)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/home/tester/.claude/CLAUDE.md",
		"/home/tester/.claude/rules/**",
		"/home/tester/AGENTS.md",
		"/home/tester/.claude/AGENTS.md",
		"/home/tester/CLAUDE.md",
		"/home/tester/CLAUDE.local.md",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("excludePaths = %q\nwant %q", got, want)
	}
	if _, err := excludePaths("relative"); err == nil {
		t.Fatal("relative home accepted")
	}
}

func TestGlobalInstructionsState(t *testing.T) {
	all := allExcludes(testHome)
	tests := []struct {
		name     string
		settings []byte
		want     profile.SettingState
	}{
		{"no file", nil, profile.SettingOn},
		{"no key", []byte(`{"env":{"A":"b"}}`), profile.SettingOn},
		{"empty list", []byte(`{"claudeMdExcludes":[]}`), profile.SettingOn},
		{"all six", settingsJSON(t, map[string]any{excludesKey: all}), profile.SettingOff},
		{"all six plus user entry", settingsJSON(t, map[string]any{excludesKey: append([]string{"/x/**"}, all...)}), profile.SettingOff},
		{"one missing", settingsJSON(t, map[string]any{excludesKey: all[1:]}), profile.SettingPartial},
		{"only user entry", settingsJSON(t, map[string]any{excludesKey: []string{"/x/**"}}), profile.SettingOn},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Adapter{}.GlobalInstructionsState(tc.settings, testHome)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.want {
				t.Fatalf("state = %q; want %q", got, tc.want)
			}
		})
	}
}

func TestGlobalInstructionsStateRejectsBadInput(t *testing.T) {
	for name, settings := range map[string][]byte{
		"not json":     []byte(`{`),
		"not a list":   []byte(`{"claudeMdExcludes":"x"}`),
		"list of nums": []byte(`{"claudeMdExcludes":[1]}`),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := (Adapter{}).GlobalInstructionsState(settings, testHome); err == nil {
				t.Fatal("state accepted bad settings")
			}
		})
	}
}

func TestMalformedExcludesRejected(t *testing.T) {
	tests := map[string]string{
		"top-level null": `{"claudeMdExcludes":null}`,
		"null element":   `{"claudeMdExcludes":[null,"/x"]}`,
		"number element": `{"claudeMdExcludes":[1]}`,
		"object":         `{"claudeMdExcludes":{"a":1}}`,
		"bare string":    `{"claudeMdExcludes":"str"}`,
	}
	for name, settings := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := (Adapter{}).GlobalInstructionsState([]byte(settings), testHome); err == nil {
				t.Fatal("state accepted malformed excludes")
			}
			for _, on := range []bool{false, true} {
				if _, err := (Adapter{}).WithGlobalInstructions([]byte(settings), testHome, on); err == nil {
					t.Fatalf("WithGlobalInstructions(on=%v) accepted malformed excludes", on)
				}
			}
		})
	}
}

func TestWithGlobalInstructions(t *testing.T) {
	all := allExcludes(testHome)
	tests := []struct {
		name      string
		current   []byte
		on        bool
		wantList  []string // nil means the key must be absent
		wantKey   bool
		wantOther map[string]string // other top-level keys that must survive, as compact JSON
	}{
		{
			name:     "off on empty settings adds all six",
			on:       false,
			wantList: all,
			wantKey:  true,
		},
		{
			name:     "off keeps user entries and order",
			current:  settingsJSON(t, map[string]any{excludesKey: []string{"/x/**"}}),
			on:       false,
			wantList: append([]string{"/x/**"}, all...),
			wantKey:  true,
		},
		{
			name:     "off on partial adds only missing",
			current:  settingsJSON(t, map[string]any{excludesKey: []string{all[0], all[2]}}),
			on:       false,
			wantList: []string{all[0], all[2], all[1], all[3], all[4], all[5]},
			wantKey:  true,
		},
		{
			name:     "off is idempotent with no duplicates",
			current:  settingsJSON(t, map[string]any{excludesKey: all}),
			on:       false,
			wantList: all,
			wantKey:  true,
		},
		{
			name:     "on removes only the six and keeps user entries",
			current:  settingsJSON(t, map[string]any{excludesKey: append([]string{"/x/**"}, all...)}),
			on:       true,
			wantList: []string{"/x/**"},
			wantKey:  true,
		},
		{
			name:    "on drops key when it ends empty",
			current: settingsJSON(t, map[string]any{excludesKey: all}),
			on:      true,
			wantKey: false,
		},
		{
			name:    "on on partial removes the rest",
			current: settingsJSON(t, map[string]any{excludesKey: all[:3]}),
			on:      true,
			wantKey: false,
		},
		{
			name:    "on on empty settings writes nothing",
			on:      true,
			wantKey: false,
		},
		{
			name:      "other keys are preserved",
			current:   []byte(`{"permissions":{"allow":["Edit"]},"env":{"A":"b"}}`),
			on:        false,
			wantList:  all,
			wantKey:   true,
			wantOther: map[string]string{"permissions": `{"allow":["Edit"]}`, "env": `{"A":"b"}`},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Adapter{}.WithGlobalInstructions(tc.current, testHome, tc.on)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				if tc.wantKey || tc.wantOther != nil || len(tc.current) > 0 {
					t.Fatalf("nil output for settings that should be kept")
				}
				return
			}
			list, has := excludesOf(t, got)
			if has != tc.wantKey {
				t.Fatalf("key present = %v; want %v in %s", has, tc.wantKey, got)
			}
			if tc.wantKey && !reflect.DeepEqual(list, tc.wantList) {
				t.Fatalf("excludes = %q; want %q", list, tc.wantList)
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(got, &m); err != nil {
				t.Fatal(err)
			}
			for k, want := range tc.wantOther {
				var a, b any
				if err := json.Unmarshal(m[k], &a); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal([]byte(want), &b); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(a, b) {
					t.Fatalf("%s = %s; want %s", k, m[k], want)
				}
			}
			// Writing the same state again must not change the bytes.
			again, err := Adapter{}.WithGlobalInstructions(got, testHome, tc.on)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(again, got) {
				t.Fatalf("not idempotent:\n%s\n%s", got, again)
			}
		})
	}
}

func TestWithGlobalInstructionsRoundTrip(t *testing.T) {
	var current []byte
	for _, want := range []profile.SettingState{profile.SettingOff, profile.SettingOn, profile.SettingOff} {
		on := want == profile.SettingOn
		next, err := Adapter{}.WithGlobalInstructions(current, testHome, on)
		if err != nil {
			t.Fatal(err)
		}
		current = next
		got, err := Adapter{}.GlobalInstructionsState(current, testHome)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("state after setting %q = %q", want, got)
		}
	}
}

func TestGlobalInstructionsPathsFollowHome(t *testing.T) {
	home := filepath.Join(t.TempDir(), "h")
	got, err := excludePaths(home)
	if err != nil {
		t.Fatal(err)
	}
	if got[0] != filepath.Join(home, ".claude", "CLAUDE.md") {
		t.Fatalf("first path = %q", got[0])
	}
}
