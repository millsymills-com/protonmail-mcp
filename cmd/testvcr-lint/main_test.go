package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
