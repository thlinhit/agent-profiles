package main

import (
	"bytes"
	"testing"
)

func TestHelp(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"help"}, &out, &out); err != nil || out.Len() == 0 {
		t.Fatalf("help: %v, %q", err, out.String())
	}
}
