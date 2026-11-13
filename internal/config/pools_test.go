package config

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

func TestPoolUpdate(t *testing.T) {
	for _, name := range []string{"add creates file", "duplicate no-op", "missing directory", "rm tilde", "rm absolute", "rm absent", "other keys preserved", "symlink refused"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "config"))
			pool := filepath.Join(home, "pool")
			if err := os.Mkdir(pool, 0755); err != nil {
				t.Fatal(err)
			}
			path, err := configPath()
			if err != nil {
				t.Fatal(err)
			}
			input, add := "~/pool", true
			var before []byte
			if name != "add creates file" && name != "missing directory" {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				before = []byte("# keep on no-op\nother = 'keep'\nskill_pools = ['~/pool']\n[nested]\nflag = true\n")
				if name == "rm tilde" {
					before = []byte("skill_pools = [" + "'" + pool + "']\n")
				}
				if err := os.WriteFile(path, before, 0600); err != nil {
					t.Fatal(err)
				}
			}
			switch name {
			case "duplicate no-op":
				input = pool
			case "missing directory":
				input = filepath.Join(home, "absent")
			case "rm tilde":
				add = false
			case "rm absolute":
				input, add = pool, false
			case "rm absent":
				input, add = filepath.Join(home, "absent"), false
			case "other keys preserved":
				input = filepath.Join(home, "second")
				if err := os.Mkdir(input, 0700); err != nil {
					t.Fatal(err)
				}
			case "symlink refused":
				target := filepath.Join(home, "target")
				if err := os.Rename(path, target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			}
			err = UpdatePool(input, add)
			wantErr := name == "missing directory" || name == "rm absent" || name == "symlink refused"
			if (err != nil) != wantErr {
				t.Fatalf("UpdatePool: %v", err)
			}
			if wantErr {
				return
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if name == "duplicate no-op" && !bytes.Equal(data, before) {
				t.Fatal("no-op rewrote config")
			}
			var values map[string]any
			if err := toml.Unmarshal(data, &values); err != nil {
				t.Fatal(err)
			}
			pools := values["skill_pools"].([]any)
			if !add && len(pools) != 0 {
				t.Fatalf("pools: %v", pools)
			}
			if name == "add creates file" && (len(pools) != 1 || pools[0] != pool) {
				t.Fatalf("pools: %v", pools)
			}
			if name == "other keys preserved" {
				if values["other"] != "keep" || values["nested"].(map[string]any)["flag"] != true {
					t.Fatalf("lost keys: %v", values)
				}
			}
		})
	}
}
