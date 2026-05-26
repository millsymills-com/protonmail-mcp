package keychain

import (
	"errors"
	"strings"
	"testing"
)

func TestDiagnoseKeychainErrNilPassesThrough(t *testing.T) {
	if got := diagnoseKeychainErr(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestDiagnoseKeychainErrFor(t *testing.T) {
	interaction := errors.New(interactionNotAllowedStatus)
	otherStatus := errors.New("exit status 37")
	backend := errors.New("some other backend error")

	tests := []struct {
		name        string
		cause       error
		goos        string
		wantAugment bool
	}{
		{"darwin interaction-not-allowed augments", interaction, "darwin", true},
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
