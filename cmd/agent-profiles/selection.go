package main

import (
	"fmt"
	"github.com/linhtruong/agent-profiles/internal/agent"
	"github.com/linhtruong/agent-profiles/internal/dotfile"
	"github.com/linhtruong/agent-profiles/internal/profile"
	"github.com/linhtruong/agent-profiles/internal/trust"
	"io"
	"os"
)

func selections(diagnostic io.Writer) (map[string]string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	doc, err := dotfile.Find(cwd)
	if err != nil {
		return nil, err
	}
	paths := map[string]string{}
	if doc == nil {
		return paths, nil
	}
	ok, err := (trust.Store{}).Trusted(doc)
	if err != nil {
		return nil, err
	}
	if !ok {
		_, err := fmt.Fprintf(diagnostic, "agent-profiles: untrusted dotfile in %q; run agent-profiles trust there\n", doc.Dir)
		return paths, err
	}
	// Parse exactly the raw bytes whose canonical-directory hash was trusted.
	profiles, err := dotfile.Parse(doc.Content)
	if err != nil {
		return nil, err
	}
	s := profile.Store{}
	for a, n := range profiles {
		if _, err := agent.Get(a); err != nil {
			return nil, err
		}
		p, err := s.Resolve(a, n)
		if err != nil {
			return nil, err
		}
		paths[a] = p
	}
	return paths, nil
}
