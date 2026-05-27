package session_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

// A genuine "not logged in" load (keychain.ErrNotFound) must map to the
// ErrNoSession login hint.
func TestClientNotFoundMapsToNoSession(t *testing.T) {
	kc := &fakeKC{loadErr: fmt.Errorf("load uid: %w", keychain.ErrNotFound)}
	s := session.New("http://invalid.test", kc)

	_, err := s.Client(context.Background())
	if err == nil {
		t.Fatal("expected error for absent session")
	}
	if !errors.Is(err, proterr.ErrNoSession) {
		t.Fatalf("want ErrNoSession, got %v", err)
	}
}

// A corrupt or unreadable credential store is not the "not logged in" state:
// Client must surface it verbatim, not tell the user to re-run login (which
// would hit the same error). Regression for the PR's headline claim.
func TestClientCorruptLoadSurfaced(t *testing.T) {
	cause := errors.New("credfile parse /x/credentials.json: invalid character 'n'")
	kc := &fakeKC{loadErr: cause}
	s := session.New("http://invalid.test", kc)

	_, err := s.Client(context.Background())
	if err == nil {
		t.Fatal("expected error for corrupt store")
	}
	if errors.Is(err, proterr.ErrNoSession) {
		t.Fatalf("corrupt-store error misreported as no-session: %v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("underlying cause not preserved: %v", err)
	}
	if !strings.Contains(err.Error(), "credential store") {
		t.Fatalf("missing context, got %v", err)
	}
}
