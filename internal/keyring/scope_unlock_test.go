package keyring_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"

	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
)

// saltsErrFetcher fails at the GetSalts step with a configurable error so the
// unlock path's scope-denial classification can be exercised. The user and
// address fetches are never reached.
type saltsErrFetcher struct{ err error }

func (f saltsErrFetcher) GetSalts(context.Context) (proton.Salts, error) {
	return nil, f.err
}
func (saltsErrFetcher) GetUser(context.Context) (proton.User, error) { return proton.User{}, nil }
func (saltsErrFetcher) GetAddresses(context.Context) ([]proton.Address, error) {
	return nil, nil
}

// TestUnlockTagsScopeDeniedSalts proves a 403/Code 9101 from GetSalts — the
// under-scoped-session failure shared by message-body and calendar decryption
// — is tagged ErrKeyringUnlockScope and maps to the actionable code.
func TestUnlockTagsScopeDeniedSalts(t *testing.T) {
	f := saltsErrFetcher{err: &proton.APIError{Status: http.StatusForbidden, Code: 9101}}

	_, err := keyring.Unlock(t.Context(), f, []byte("pw"))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, proterr.ErrKeyringUnlockScope) {
		t.Fatalf("error not tagged ErrKeyringUnlockScope: %v", err)
	}
	if pe := proterr.Map(err); pe == nil || pe.Code != "proton/keyring_unlock_scope" {
		t.Fatalf("Map = %+v, want proton/keyring_unlock_scope", pe)
	}
}

// TestUnlockDoesNotTagNonScopeSaltsFailure proves an unrelated salts failure
// (e.g. a generic 403 or transport error) is NOT misattributed to the scope
// sentinel, so genuine denials keep their own mapping.
func TestUnlockDoesNotTagNonScopeSaltsFailure(t *testing.T) {
	f := saltsErrFetcher{err: &proton.APIError{Status: http.StatusForbidden, Code: 2001}}

	_, err := keyring.Unlock(t.Context(), f, []byte("pw"))
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, proterr.ErrKeyringUnlockScope) {
		t.Fatalf("non-scope 403 wrongly tagged as scope denial: %v", err)
	}
	if pe := proterr.Map(err); pe == nil || pe.Code != "proton/permission_denied" {
		t.Fatalf("Map = %+v, want proton/permission_denied", pe)
	}
}
