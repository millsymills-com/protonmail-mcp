package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func TestStatusReportsFileBackend(t *testing.T) {
	keyring.MockInit()
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

func TestStatusProbesOtherBackendWhenSessionElsewhere(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	// Seed a session under the file backend, then run status with the default
	// (keychain) backend so the env points at no session on the resolved one.
	fileStore, err := session.SelectStore(func(k string) string {
		switch k {
		case "PROTONMAIL_MCP_CREDENTIAL_BACKEND":
			return "file"
		case "PROTONMAIL_MCP_STATE_DIR":
			return dir
		default:
			return ""
		}
	})
	if err != nil {
		t.Fatalf("seed store: %v", err)
	}
	if err := fileStore.SaveSession(keychain.Session{UID: "u", AccessToken: "a", RefreshToken: "r"}); err != nil {
		t.Fatalf("seed session: %v", err)
	}

	var out bytes.Buffer
	code := run(context.Background(), []string{"status"},
		[]string{"PROTONMAIL_MCP_STATE_DIR=" + dir},
		strings.NewReader(""), &out, &out, nil)
	if code != 0 {
		t.Fatalf("status exit code = %d, want 0; out: %s", code, out.String())
	}
	got := out.String()
	if strings.Contains(got, "\nnot logged in\n") {
		t.Fatalf("status should surface the cross-backend hint, not bare not-logged-in; got: %s", got)
	}
	if !strings.Contains(got, "a file-backend session exists at "+dir) {
		t.Fatalf("status missing file-backend hint; got: %s", got)
	}
	if !strings.Contains(got, "PROTONMAIL_MCP_CREDENTIAL_BACKEND=file") {
		t.Fatalf("status missing env-change hint; got: %s", got)
	}
}

func TestStatusReportsPlainNotLoggedInWhenNoSessionAnywhere(t *testing.T) {
	keyring.MockInit()
	dir := t.TempDir()
	var out bytes.Buffer
	code := run(context.Background(), []string{"status"},
		[]string{"PROTONMAIL_MCP_STATE_DIR=" + dir},
		strings.NewReader(""), &out, &out, nil)
	if code != 0 {
		t.Fatalf("status exit code = %d, want 0; out: %s", code, out.String())
	}
	got := out.String()
	if !strings.Contains(got, "not logged in") {
		t.Fatalf("expected plain not-logged-in; got: %s", got)
	}
	if strings.Contains(got, "a file-backend session exists") {
		t.Fatalf("must not print a cross-backend hint when no session anywhere; got: %s", got)
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
