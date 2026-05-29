package keychain

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"

	dbus "github.com/godbus/dbus/v5"
)

func TestDiagnoseKeychainErrNilPassesThrough(t *testing.T) {
	if got := diagnoseKeychainErr(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

// exitErrWithCode runs a child process that exits with the given code and
// returns the resulting *exec.ExitError, mirroring what go-keyring surfaces
// when /usr/bin/security exits non-zero. Using a real exec error (rather than
// errors.New) exercises the structural errors.As path the diagnosis relies on.
func exitErrWithCode(t *testing.T, code int) *exec.ExitError {
	t.Helper()
	err := exec.Command("sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected *exec.ExitError from exit %d, got %T: %v", code, err, err)
	}
	if exitErr.ExitCode() != code {
		t.Fatalf("expected exit code %d, got %d", code, exitErr.ExitCode())
	}
	return exitErr
}

// dbusErrNamed builds a dbus.Error as go-keyring surfaces it from a failed
// Secret Service call, so the test exercises the structural errors.As path.
func dbusErrNamed(name, body string) dbus.Error {
	return dbus.Error{Name: name, Body: []any{body}}
}

func TestDiagnoseKeychainErrFor(t *testing.T) {
	interaction := exitErrWithCode(t, interactionNotAllowedExitCode)
	wrapped := fmt.Errorf("set password: %w", exitErrWithCode(t, interactionNotAllowedExitCode))
	otherStatus := exitErrWithCode(t, 37)
	backend := errors.New("some other backend error")
	noSecretService := dbusErrNamed(
		secretServiceUnknownDBusName,
		"The name org.freedesktop.secrets was not provided by any .service files")
	wrappedNoSecretService := fmt.Errorf("save username: %w", noSecretService)
	otherDBus := dbusErrNamed("org.freedesktop.DBus.Error.AccessDenied", "denied")

	tests := []struct {
		name        string
		cause       error
		goos        string
		wantAugment bool
		wantHint    string
	}{
		{"darwin interaction-not-allowed augments", interaction, "darwin", true, "unlock-keychain"},
		{"darwin wrapped interaction-not-allowed augments", wrapped, "darwin", true, "unlock-keychain"},
		{"darwin other exit status passes through", otherStatus, "darwin", false, ""},
		{"darwin non-exit error passes through", backend, "darwin", false, ""},
		{"darwin secret-service-unknown passes through", noSecretService, "darwin", false, ""},
		{"linux interaction-not-allowed passes through", interaction, "linux", false, ""},
		{"linux backend error passes through", backend, "linux", false, ""},
		{"linux secret-service-unknown augments", noSecretService, "linux", true, "CREDENTIAL_BACKEND=file"},
		{"linux wrapped secret-service-unknown augments", wrappedNoSecretService, "linux", true, "CREDENTIAL_BACKEND=file"},
		{"linux other dbus error passes through", otherDBus, "linux", false, ""},
		{"nil darwin", nil, "darwin", false, ""},
		{"nil linux", nil, "linux", false, ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := diagnoseKeychainErrFor(tc.cause, tc.goos)

			if tc.cause == nil {
				if got != nil {
					t.Fatalf("expected nil, got %v", got)
				}
				return
			}
			// dbus.Error is non-comparable (slice Body), so errors.Is can't match
			// it by identity; assert the wrap stays errors.As-recoverable instead,
			// since downstream callers rely on that to classify the failure.
			var causeDBus dbus.Error
			if errors.As(tc.cause, &causeDBus) {
				var gotDBus dbus.Error
				if !errors.As(got, &gotDBus) || gotDBus.Name != causeDBus.Name {
					t.Fatalf("dbus cause must remain recoverable via errors.As; got %v", got)
				}
			} else if !errors.Is(got, tc.cause) {
				t.Fatalf("result must wrap the original cause; got %v", got)
			}
			if !strings.Contains(got.Error(), tc.cause.Error()) {
				t.Fatalf("result must preserve original message; got %q", got.Error())
			}

			augmented := got.Error() != tc.cause.Error()
			if augmented != tc.wantAugment {
				t.Fatalf("augmented=%v, want %v (got %q)", augmented, tc.wantAugment, got.Error())
			}
			if tc.wantAugment && !strings.Contains(got.Error(), tc.wantHint) {
				t.Fatalf("augmented error must carry hint %q; got %q", tc.wantHint, got.Error())
			}
		})
	}
}
