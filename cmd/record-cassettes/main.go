//go:build recording

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/millsmillsymills/protonmail-mcp/cmd/record-cassettes/scenarios"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: record-cassettes <scenario>")
		fmt.Fprintln(os.Stderr, "available scenarios:")
		for _, name := range scenarios.Names() {
			fmt.Fprintf(os.Stderr, "  - %s\n", name)
		}
		os.Exit(2)
	}
	name := os.Args[1]
	fn, ok := scenarios.Lookup(name)
	if !ok {
		fmt.Fprintf(os.Stderr, "unknown scenario %q\n", name)
		os.Exit(2)
	}
	if err := os.Setenv("VCR_MODE", "record"); err != nil {
		panic(err)
	}
	ctx := context.Background()
	if err := fn(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
		os.Exit(1)
	}
}
