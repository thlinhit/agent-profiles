package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

func confirmDelete(input *os.File, diagnostic io.Writer, agent, name string) error {
	if !term.IsTerminal(int(input.Fd())) {
		return fmt.Errorf("delete requires --yes when stdin is not a terminal")
	}
	if _, err := fmt.Fprintf(diagnostic, "Delete profile %q for %q? [y/N] ", name, agent); err != nil {
		return err
	}
	answer, err := bufio.NewReader(io.LimitReader(input, 1024)).ReadString('\n')
	if err != nil {
		if err == io.EOF {
			return fmt.Errorf("deletion cancelled")
		}
		return fmt.Errorf("read confirmation: %w", err)
	}
	switch strings.ToLower(strings.TrimSpace(answer)) {
	case "y", "yes":
		return nil
	default:
		return fmt.Errorf("deletion cancelled")
	}
}
