package keychain

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/zalando/go-keyring"
)

// failOnNth wraps the keyring seam funcs so a chosen op ordinal returns
// failErr while every other op delegates to the real (mock) store. This
// reaches the intermediate Save*/Load* error-wrap branches that go-keyring's
// all-or-nothing mock cannot: a later op failing after earlier ones succeed.
type failOnNth struct {
	setCalls, getCalls, delCalls int
	failSetN, failGetN, failDelN int
	failErr                      error

	origSet func(service, user, password string) error
	origGet func(service, user string) (string, error)
	origDel func(service, user string) error
}

func installFailOnNth(t *testing.T, f *failOnNth) {
	t.Helper()
	keyring.MockInit()
	f.origSet, f.origGet, f.origDel = keyringSet, keyringGet, keyringDelete
	t.Cleanup(func() {
		keyringSet, keyringGet, keyringDelete = f.origSet, f.origGet, f.origDel
	})
	keyringSet = func(service, user, password string) error {
		f.setCalls++
		if f.setCalls == f.failSetN {
			return f.failErr
		}
		return f.origSet(service, user, password)
	}
	keyringGet = func(service, user string) (string, error) {
		f.getCalls++
		if f.getCalls == f.failGetN {
			return "", f.failErr
		}
		return f.origGet(service, user)
	}
	keyringDelete = func(service, user string) error {
		f.delCalls++
		if f.delCalls == f.failDelN {
			return f.failErr
		}
		return f.origDel(service, user)
	}
}

func TestSaveIntermediateErrors(t *testing.T) {
	boom := errors.New("backend fault")
	tests := []struct {
		name   string
		fail   failOnNth
		run    func(*Keychain) error
		wantIn string
	}{
		{
			"creds-password-set", failOnNth{failSetN: 2, failErr: boom},
			func(k *Keychain) error {
				return k.SaveCreds(Creds{Username: "u", Password: "p", TOTPSecret: "t"})
			}, "save password",
		},
		{
			"creds-totp-set", failOnNth{failSetN: 3, failErr: boom},
			func(k *Keychain) error {
				return k.SaveCreds(Creds{Username: "u", Password: "p", TOTPSecret: "t"})
			}, "save totp",
		},
		{
			"creds-stale-totp-delete", failOnNth{failDelN: 1, failErr: boom},
			func(k *Keychain) error {
				return k.SaveCreds(Creds{Username: "u", Password: "p"})
			}, "clear stale totp",
		},
		{
			"session-access-set", failOnNth{failSetN: 2, failErr: boom},
			func(k *Keychain) error {
				return k.SaveSession(Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
			}, "save access token",
		},
		{
			"session-refresh-set", failOnNth{failSetN: 3, failErr: boom},
			func(k *Keychain) error {
				return k.SaveSession(Session{UID: "u", AccessToken: "a", RefreshToken: "r"})
			}, "save refresh token",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.fail
			installFailOnNth(t, &f)
			err := tc.run(New())
			if err == nil || !strings.Contains(err.Error(), tc.wantIn) {
				t.Fatalf("want %q error, got %v", tc.wantIn, err)
			}
		})
	}
}

// TestLoadCredsTOTPGetError covers keychain.go:81 — a TOTP Get failure that is
// not ErrNotFound surfaces as a load error rather than being tolerated. The
// username and password Gets must succeed first, so seed them before failing
// the third Get.
func TestLoadCredsTOTPGetError(t *testing.T) {
	f := failOnNth{failGetN: 3, failErr: errors.New("backend fault")}
	installFailOnNth(t, &f)
	if err := keyringSet(service, keyUsername, "u"); err != nil {
		t.Fatalf("seed username: %v", err)
	}
	if err := keyringSet(service, keyPassword, "p"); err != nil {
		t.Fatalf("seed password: %v", err)
	}
	_, err := New().LoadCreds()
	if err == nil || !strings.Contains(err.Error(), "load totp") {
		t.Fatalf("want load-totp error, got %v", err)
	}
}

// TestLoadCredsInteractionNotAllowedHint locks in that the read path is wired to
// diagnoseKeychainErr. A locked-keychain secret read surfaces *exec.ExitError
// code 36 just like the write path, so the first Get failing with code 36 must
// reach the caller as a load error that, on darwin, carries the unlock hint.
func TestLoadCredsInteractionNotAllowedHint(t *testing.T) {
	f := failOnNth{failGetN: 1, failErr: exitErrWithCode(t, interactionNotAllowedExitCode)}
	installFailOnNth(t, &f)
	_, err := New().LoadCreds()
	if err == nil || !strings.Contains(err.Error(), "load username") {
		t.Fatalf("want load-username error, got %v", err)
	}
	if runtime.GOOS == "darwin" && !strings.Contains(err.Error(), "unlock-keychain") {
		t.Fatalf("darwin load failure should carry the unlock hint, got %v", err)
	}
}
