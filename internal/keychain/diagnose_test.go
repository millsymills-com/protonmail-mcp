package keychain

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
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

func TestDiagnoseKeychainErrFor(t *testing.T) {
	interaction := exitErrWithCode(t, interactionNotAllowedExitCode)
	wrapped := fmt.Errorf("set password: %w", exitErrWithCode(t, interactionNotAllowedExitCode))
	otherStatus := exitErrWithCode(t, 37)
	backend := errors.New("some other backend error")

	tests := []struct {
		name        string
		cause       error
		goos        string
		wantAugment bool
	}{
		{"darwin interaction-not-allowed augments", interaction, "darwin", true},
		{"darwin wrapped interaction-not-allowed augments", wrapped, "darwin", true},
		{"darwin other exit status passes through", otherStatus, "darwin", false},
		{"darwin non-exit error passes through", backend, "darwin", false},
		{"linux interaction-not-allowed passes through", interaction, "linux", false},
		{"linux backend error passes through", backend, "linux", false},
		{"nil darwin", nil, "darwin", false},
		{"nil linux", nil, "linux", false},
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
			if !errors.Is(got, tc.cause) {
				t.Fatalf("result must wrap the original cause; got %v", got)
			}
			if !strings.Contains(got.Error(), tc.cause.Error()) {
				t.Fatalf("result must preserve original message; got %q", got.Error())
			}

			augmented := got.Error() != tc.cause.Error()
			if augmented != tc.wantAugment {
				t.Fatalf("augmented=%v, want %v (got %q)", augmented, tc.wantAugment, got.Error())
			}
			if tc.wantAugment && !strings.Contains(got.Error(), "unlock-keychain") {
				t.Fatalf("augmented error must carry the unlock hint; got %q", got.Error())
			}
		})
	}
}
