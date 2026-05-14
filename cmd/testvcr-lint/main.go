package main

import (
	"fmt"
	"os"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

var softRules = map[string]bool{
	"stale-cassette": true,
	"version-drift":  true,
}

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		// Keep this list in sync when adding new cassette directories.
		roots = []string{
			"internal/tools/testdata/cassettes",
			"internal/session/testdata/cassettes",
			"internal/server/testdata/cassettes",
			"internal/testharness/testdata/cassettes",
			"internal/testvcr/testdata/cassettes",
			"cmd/protonmail-mcp/testdata/cassettes",
		}
	}
	findings := testvcr.Scan(roots...)
	if len(findings) == 0 {
		return
	}
	strict := os.Getenv("STRICT") == "1"
	hardErr := false
	for _, f := range findings {
		fmt.Fprintf(os.Stderr, "%s:%d [%s] %s\n", f.Path, f.Line, f.Rule, f.Hit)
		if !softRules[f.Rule] {
			hardErr = true
		}
	}
	if hardErr || strict {
		os.Exit(1)
	}
}
