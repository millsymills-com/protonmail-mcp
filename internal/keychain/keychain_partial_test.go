package keychain

import (
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// A partially-populated keychain (one entry written, a later one missing)
// must surface a load error rather than returning a half-built bundle. The
// go-keyring mock can't fail an individual Set, so these paths are reached
// by seeding only the leading entries and letting the missing Get fail.

func TestLoadCredsPasswordMissing(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set(service, keyUsername, "u"); err != nil {
		t.Fatalf("seed username: %v", err)
	}
	_, err := New().LoadCreds()
	if err == nil || !strings.Contains(err.Error(), "load password") {
		t.Fatalf("want load-password error, got %v", err)
	}
}

func TestLoadSessionAccessTokenMissing(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set(service, keyUID, "u"); err != nil {
		t.Fatalf("seed uid: %v", err)
	}
	_, err := New().LoadSession()
	if err == nil || !strings.Contains(err.Error(), "load access token") {
		t.Fatalf("want load-access-token error, got %v", err)
	}
}

func TestLoadSessionRefreshTokenMissing(t *testing.T) {
	keyring.MockInit()
	if err := keyring.Set(service, keyUID, "u"); err != nil {
		t.Fatalf("seed uid: %v", err)
	}
	if err := keyring.Set(service, keyAccessToken, "a"); err != nil {
		t.Fatalf("seed access token: %v", err)
	}
	_, err := New().LoadSession()
	if err == nil || !strings.Contains(err.Error(), "load refresh token") {
		t.Fatalf("want load-refresh-token error, got %v", err)
	}
}
