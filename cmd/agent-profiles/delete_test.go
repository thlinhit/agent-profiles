package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"

	"github.com/thlinhit/agent-profiles/internal/profile"
)

func TestDeleteInteractiveConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name, answer string
		deleted      bool
	}{
		{"yes", "y\n", true}, {"yes_word", "yes\n", true}, {"no", "n\n", false}, {"empty_answer", "\n", false}, {"not_yes", "yesterday\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
			s := profile.Store{}
			path, err := s.Create("claude", "work", nil)
			if err != nil {
				t.Fatal(err)
			}
			master, terminal, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = master.Close(); _ = terminal.Close() }()
			stdin := os.Stdin
			os.Stdin = terminal
			t.Cleanup(func() { os.Stdin = stdin })
			if _, err := io.WriteString(master, tc.answer); err != nil {
				t.Fatal(err)
			}
			var out, diagnostic bytes.Buffer
			err = run([]string{"delete", "claude", "work"}, &out, &diagnostic)
			if tc.deleted && err != nil {
				t.Fatal(err)
			}
			if !tc.deleted && err == nil {
				t.Fatal("expected cancellation")
			}
			if out.Len() != 0 || !strings.Contains(diagnostic.String(), "[y/N]") {
				t.Fatalf("prompt on wrong channel or missing: %q %q", out.String(), diagnostic.String())
			}
			_, err = os.Lstat(path)
			if tc.deleted && !os.IsNotExist(err) {
				t.Fatalf("profile not deleted: %v", err)
			}
			if !tc.deleted && err != nil {
				t.Fatalf("cancelled deletion changed profile: %v", err)
			}
		})
	}
}
func TestDeleteNonTTYRefusal(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	s := profile.Store{}
	path, err := s.Create("claude", "work", nil)
	if err != nil {
		t.Fatal(err)
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = r.Close(); _ = w.Close() }()
	if _, err := io.WriteString(w, "yes\n"); err != nil {
		t.Fatal(err)
	}
	stdin := os.Stdin
	os.Stdin = r
	t.Cleanup(func() { os.Stdin = stdin })
	var out, diagnostic bytes.Buffer
	err = run([]string{"delete", "claude", "work"}, &out, &diagnostic)
	if err == nil || !strings.Contains(err.Error(), "--yes") || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("unclear refusal: %v", err)
	}
	if diagnostic.Len() != 0 {
		t.Fatal("prompted on non-TTY")
	}
	if _, err := s.Resolve("claude", "work"); err != nil {
		t.Fatal("profile deleted")
	}
	if err := run([]string{"delete", "claude", "work", "--yes"}, &out, &diagnostic); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("--yes did not delete: %v", err)
	}
}
