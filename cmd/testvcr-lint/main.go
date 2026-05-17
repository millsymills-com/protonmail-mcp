package main

import (
	"fmt"
	"io"
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
	if classifyExitCode(findings, os.Getenv("STRICT") == "1", os.Stderr) != 0 {
		os.Exit(1)
	}
}

// classifyExitCode prints findings to out and returns the process exit code:
//   - 0 when there are no findings, or when only soft-rule findings exist and
//     strict is false;
//   - 1 when any hard-rule finding is present, or when strict=true and any
//     finding (hard or soft) is present.
//
// Soft rules (currently `stale-cassette` and `version-drift`) are advisory
// because they depend on time/version drift rather than secrets leaking into
// cassettes. STRICT=1 promotes them to errors so CI gates and pre-release
// runs can require a fully fresh cassette tree.
func classifyExitCode(findings []testvcr.Finding, strict bool, out io.Writer) int {
	if len(findings) == 0 {
		return 0
	}
	hardErr := false
	for _, f := range findings {
		_, _ = fmt.Fprintf(out, "%s:%d [%s] %s\n", f.Path, f.Line, f.Rule, f.Hit)
		if !softRules[f.Rule] {
			hardErr = true
		}
	}
	if hardErr || strict {
		return 1
	}
	return 0
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
