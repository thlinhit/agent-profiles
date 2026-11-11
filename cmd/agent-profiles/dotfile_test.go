package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalCommand(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	t.Chdir(home)
	var out bytes.Buffer
	if err := run([]string{"create", "claude", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"local", "claude", "work"}, &out, &out); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(home, ".agent-profile"))
	if err != nil || !bytes.Contains(b, []byte("work")) {
		t.Fatalf("local: %q %v", b, err)
	}
	if err := run([]string{"local", "claude", "missing"}, &out, &out); err == nil {
		t.Fatal("missing profile accepted")
	}
}
