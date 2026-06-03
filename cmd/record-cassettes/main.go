//go:build recording

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/millsymills-com/protonmail-mcp/cmd/record-cassettes/scenarios"
)

// main accepts one or more scenario names and runs them sequentially in a
// single process. Multiple scenarios share one login via the in-process
// session cache in scenarios/auth.go so a 30-scenario batch costs one SRP
// exchange (not 30) — keeps Proton's anti-abuse threshold from firing.
func main() {
	scenarios.RegisterAll()
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: record-cassettes <scenario> [scenario...]")
		fmt.Fprintln(os.Stderr, "available scenarios:")
		for _, name := range scenarios.Names() {
			fmt.Fprintf(os.Stderr, "  - %s\n", name)
		}
		os.Exit(2)
	}
	if err := os.Setenv("VCR_MODE", "record"); err != nil {
		panic(err)
	}
	ctx := context.Background()
	failed := false
	for _, name := range os.Args[1:] {
		fn, ok := scenarios.Lookup(name)
		if !ok {
			fmt.Fprintf(os.Stderr, "unknown scenario %q\n", name)
			failed = true
			continue
		}
		start := time.Now()
		err := fn(ctx)
		dur := time.Since(start).Round(time.Millisecond)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[FAIL %s] %s: %v\n", dur, name, err)
			failed = true
		} else {
			fmt.Fprintf(os.Stderr, "[PASS %s] %s\n", dur, name)
		}
	}
	scenarios.FinalLogout()
	if failed {
		os.Exit(1)
	}
}
