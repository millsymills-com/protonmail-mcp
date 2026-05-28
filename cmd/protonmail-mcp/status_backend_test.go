package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestStatusReportsFileBackend(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	// File backend with an empty state dir: status reports the backend, prints
	// "not logged in", and exits 0 (informational command).
	code := run(context.Background(), []string{"status"},
		[]string{"PROTONMAIL_MCP_CREDENTIAL_BACKEND=file", "PROTONMAIL_MCP_STATE_DIR=" + dir},
		strings.NewReader(""), &out, &out, nil)
	if code != 0 {
		t.Fatalf("status exit code = %d, want 0; out: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "backend: file") {
		t.Fatalf("status did not report file backend; got: %s", out.String())
	}
	if !strings.Contains(out.String(), "not logged in") {
		t.Fatalf("status did not report not-logged-in for empty file backend; got: %s", out.String())
	}
}

func TestStatusReportsKeychainBackendByDefault(t *testing.T) {
	keyring.MockInit()
	var out bytes.Buffer
	code := run(context.Background(), []string{"status"}, nil, strings.NewReader(""), &out, &out, nil)
	if code != 0 {
		t.Fatalf("status exit code = %d, want 0; out: %s", code, out.String())
	}
	if !strings.Contains(out.String(), "backend: keychain") {
		t.Fatalf("status did not report keychain default; got: %s", out.String())
	}
}
