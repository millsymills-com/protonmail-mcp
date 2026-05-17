package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestFindCassetteRootsDiscoversNestedDirs(t *testing.T) {
	base := t.TempDir()
	dirs := []string{
		filepath.Join("internal", "tools", "testdata", "cassettes"),
		filepath.Join("internal", "newpkg", "testdata", "cassettes"),
		filepath.Join("cmd", "protonmail-mcp", "testdata", "cassettes"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(base, "internal", "tools", "testdata", "other"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := findCassetteRoots(base)

	want := []string{
		filepath.Join(base, "cmd", "protonmail-mcp", "testdata", "cassettes"),
		filepath.Join(base, "internal", "newpkg", "testdata", "cassettes"),
		filepath.Join(base, "internal", "tools", "testdata", "cassettes"),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findCassetteRoots = %v, want %v", got, want)
	}
}

func TestFindCassetteRootsSkipsPrunedDirs(t *testing.T) {
	base := t.TempDir()
	pruned := []string{
		filepath.Join(".git", "internal", "testdata", "cassettes"),
		filepath.Join("vendor", "x", "testdata", "cassettes"),
		filepath.Join("node_modules", "y", "testdata", "cassettes"),
	}
	for _, d := range pruned {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	live := filepath.Join("internal", "tools", "testdata", "cassettes")
	if err := os.MkdirAll(filepath.Join(base, live), 0o755); err != nil {
		t.Fatal(err)
	}

	got := findCassetteRoots(base)

	want := []string{filepath.Join(base, live)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("findCassetteRoots = %v, want %v", got, want)
	}
}

func TestFindCassetteRootsEmptyTree(t *testing.T) {
	base := t.TempDir()
	if got := findCassetteRoots(base); len(got) != 0 {
		t.Fatalf("expected no roots, got %v", got)
	}
}

func TestClassifyExitCodeNoFindings(t *testing.T) {
	for _, strict := range []bool{false, true} {
		out := &bytes.Buffer{}
		got := classifyExitCode(nil, strict, out)
		if got != 0 {
			t.Fatalf("strict=%v: code = %d, want 0", strict, got)
		}
		if out.Len() != 0 {
			t.Fatalf("strict=%v: unexpected output %q", strict, out.String())
		}
	}
}

func TestClassifyExitCodeSoftOnly(t *testing.T) {
	findings := []testvcr.Finding{
		{Path: "a.yaml", Line: 1, Rule: "stale-cassette", Hit: "old"},
		{Path: "b.yaml", Line: 2, Rule: "version-drift", Hit: "v1 vs v2"},
	}

	out := &bytes.Buffer{}
	if got := classifyExitCode(findings, false, out); got != 0 {
		t.Fatalf("strict=false: code = %d, want 0 (soft findings should not fail)", got)
	}
	// Findings must still be reported on out regardless of exit code.
	if !strings.Contains(out.String(), "stale-cassette") || !strings.Contains(out.String(), "version-drift") {
		t.Fatalf("expected soft findings on out; got %q", out.String())
	}

	outStrict := &bytes.Buffer{}
	if got := classifyExitCode(findings, true, outStrict); got != 1 {
		t.Fatalf("strict=true: code = %d, want 1 (STRICT must promote soft findings)", got)
	}
}

func TestClassifyExitCodeHardAlwaysFails(t *testing.T) {
	findings := []testvcr.Finding{
		{Path: "a.yaml", Line: 5, Rule: "bearer-token", Hit: "Bearer abc..."},
	}
	for _, strict := range []bool{false, true} {
		out := &bytes.Buffer{}
		if got := classifyExitCode(findings, strict, out); got != 1 {
			t.Fatalf("strict=%v: code = %d, want 1 (hard rule must fail in both modes)", strict, got)
		}
	}
}

func TestClassifyExitCodeMixedSoftAndHardAlwaysFails(t *testing.T) {
	findings := []testvcr.Finding{
		{Path: "a.yaml", Line: 1, Rule: "stale-cassette", Hit: "old"},
		{Path: "b.yaml", Line: 5, Rule: "bearer-token", Hit: "Bearer abc..."},
	}
	for _, strict := range []bool{false, true} {
		out := &bytes.Buffer{}
		if got := classifyExitCode(findings, strict, out); got != 1 {
			t.Fatalf("strict=%v: code = %d, want 1", strict, got)
		}
	}
}
