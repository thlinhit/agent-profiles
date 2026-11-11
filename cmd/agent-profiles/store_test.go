package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreCommands(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", filepath.Join(os.Getenv("HOME"), "data"))
	for _, args := range [][]string{{"create", "claude", "work", "--init"}, {"list", "claude"}, {"delete", "claude", "work", "--yes"}} {
		var out bytes.Buffer
		if err := run(args, &out, &out); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	var out bytes.Buffer
	if err := run([]string{"delete", "claude", "work"}, &out, &out); err == nil {
		t.Fatal("delete without confirmation accepted")
	}
}
