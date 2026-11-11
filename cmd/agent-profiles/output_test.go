package main

import (
	"errors"
	"io"
	"path/filepath"
	"testing"
)

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }
func TestOutputErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", filepath.Join(home, "data"))
	if err := run([]string{"create", "claude", "work"}, io.Discard, io.Discard); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"list"}, {"current"}} {
		if err := run(args, failedWriter{}, io.Discard); !errors.Is(err, io.ErrClosedPipe) {
			t.Fatalf("%v discarded write failure: %v", args, err)
		}
	}
}
