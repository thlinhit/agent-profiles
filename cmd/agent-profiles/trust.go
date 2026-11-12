package main

import (
	"fmt"
	"os"

	"github.com/thlinhit/agent-profiles/internal/dotfile"
	"github.com/thlinhit/agent-profiles/internal/trust"
)

func runTrust(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: %s", args[0])
	}
	dir, err := os.Getwd()
	if err != nil {
		return err
	}
	dir, err = dotfile.Canonical(dir)
	if err != nil {
		return err
	}
	if args[0] == "untrust" {
		return (trust.Store{}).Revoke(dir)
	}
	doc, err := dotfile.Find(dir)
	if err != nil {
		return err
	}
	if doc == nil || doc.Dir != dir {
		return fmt.Errorf("trust requires a dotfile in the current directory")
	}
	// Approval binds raw bytes only; resolution validates TOML after trust.
	return (trust.Store{}).Grant(doc)
}
