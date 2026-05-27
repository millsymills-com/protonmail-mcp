package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestStatusReportsFileBackend(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	_ = run(context.Background(), []string{"status"},
		[]string{"PROTONMAIL_MCP_CREDENTIAL_BACKEND=file", "PROTONMAIL_MCP_STATE_DIR=" + dir},
		strings.NewReader(""), &out, &out, nil)
	if !strings.Contains(out.String(), "backend: file") {
		t.Fatalf("status did not report backend; got: %s", out.String())
	}
}
