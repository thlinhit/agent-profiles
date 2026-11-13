package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name     string
		fallback bool
		content  *string
		want     []string
		wantErr  bool
	}{
		{name: "missing file"},
		{name: "expansion", content: strptr("skill_pools = [\"~/skills\", \"/opt/skills\", \"relative/skills\"]"), want: []string{"HOME/skills", "/opt/skills", "relative/skills"}},
		{name: "malformed TOML", content: strptr("skill_pools = ["), wantErr: true},
		{name: "wrong type", content: strptr("skill_pools = 42"), wantErr: true},
		{name: "fallback config", fallback: true, content: strptr("skill_pools = [\"~/fallback\"]"), want: []string{"HOME/fallback"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			xdg := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", xdg)
			base := xdg
			if tt.fallback {
				t.Setenv("XDG_CONFIG_HOME", "")
				base = filepath.Join(home, ".config")
			}
			if tt.content != nil {
				dir := filepath.Join(base, "agent-profiles")
				if err := os.MkdirAll(dir, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(*tt.content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			got, err := Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			want := append([]string(nil), tt.want...)
			for i, path := range want {
				if len(path) >= 5 && path[:5] == "HOME/" {
					want[i] = filepath.Join(home, path[5:])
				}
			}
			if !reflect.DeepEqual(got.SkillPools, want) {
				t.Errorf("SkillPools = %#v, want %#v", got.SkillPools, want)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, tt := range []struct{ input, want string }{
		{"~/skills", filepath.Join(home, "skills")},
		{"~/", home},
		{"~", "~"},
		{"~someone/skills", "~someone/skills"},
		{"relative/~/skills", "relative/~/skills"},
		{"/opt/skills", "/opt/skills"},
		{"", ""},
	} {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ExpandHome(tt.input)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("ExpandHome(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
	t.Run("missing home", func(t *testing.T) {
		t.Setenv("HOME", "")
		if _, err := ExpandHome("~/skills"); err == nil {
			t.Error("expected error for missing home")
		}
		if got, err := ExpandHome("relative"); err != nil || got != "relative" {
			t.Errorf("non-home path = %q, %v", got, err)
		}
	})
}

func strptr(s string) *string { return &s }
