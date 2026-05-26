package main

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

// errReader fails every Read with a non-EOF error, exercising the read-error
// branches that the EOF-based tests can't reach.
type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }

func TestPromptReaderNonEOFError(t *testing.T) {
	r := bufio.NewReader(errReader{err: errors.New("prompt read failed")})
	_, err := promptReader(&bytes.Buffer{}, r, "label: ")
	if err == nil || !strings.Contains(err.Error(), "prompt read failed") {
		t.Fatalf("want prompt read error, got %v", err)
	}
}

func TestReadPasswordNonEOFError(t *testing.T) {
	stdin := errReader{err: errors.New("password read failed")}
	fallback := bufio.NewReader(stdin)
	_, err := readPassword(&bytes.Buffer{}, stdin, fallback)
	if err == nil || !strings.Contains(err.Error(), "password read failed") {
		t.Fatalf("want password read error, got %v", err)
	}
}
