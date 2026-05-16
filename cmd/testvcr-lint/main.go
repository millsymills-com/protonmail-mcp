package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

var softRules = map[string]bool{
	"stale-cassette": true,
	"version-drift":  true,
}

// pruneDirs are walk-time prune entries by directory name. Skipping these keeps
// `make verify-cassettes` fast on large checkouts and avoids matching cassettes
// vendored from third-party modules.
var pruneDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"vendor":       true,
}

func main() {
	roots := os.Args[1:]
	if len(roots) == 0 {
		roots = findCassetteRoots(".")
	}
	if len(roots) == 0 {
		return
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

// findCassetteRoots walks base looking for any directory whose path ends in
// testdata/cassettes. Discovered roots replace the previous hardcoded list so
// new cassette directories are scanned without editing this file.
func findCassetteRoots(base string) []string {
	var roots []string
	suffix := filepath.Join("testdata", "cassettes")
	_ = filepath.WalkDir(base, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if pruneDirs[d.Name()] {
			return fs.SkipDir
		}
		if strings.HasSuffix(path, suffix) {
			roots = append(roots, path)
			return fs.SkipDir
		}
		return nil
	})
	sort.Strings(roots)
	return roots
}
