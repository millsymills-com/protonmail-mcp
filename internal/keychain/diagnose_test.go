package keychain

import (
	"errors"
	"runtime"
	"strings"
	"testing"
)

func TestDiagnoseKeychainErrNonExitStatusPassesThrough(t *testing.T) {
	in := errors.New("some other backend error")
	out := diagnoseKeychainErr(in)
	if out.Error() != in.Error() {
		t.Fatalf("expected pass-through, got %q", out.Error())
	}
}

func TestDiagnoseKeychainErrNilPassesThrough(t *testing.T) {
	if got := diagnoseKeychainErr(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestDiagnoseKeychainErrExitStatusAugmentsOnDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("darwin-only behavior")
	}
	// We can't deterministically force the keychain into a locked state
	// from a test, but the probe must always either pass-through unchanged
	// or augment with a non-empty wrapper — never panic, never lose the
	// original.
	in := errors.New("exit status 36")
	out := diagnoseKeychainErr(in)
	if !errors.Is(out, in) {
		t.Fatalf("wrapped error must Is the original; got %v", out)
	}
	if !strings.Contains(out.Error(), "exit status 36") {
		t.Fatalf("wrapped error must preserve original message; got %q", out.Error())
	}
}
