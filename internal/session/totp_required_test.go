package session

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrTOTPRequiredSurvivesWrapping(t *testing.T) {
	wrapped := fmt.Errorf("login flow: %w", ErrTOTPRequired)
	if !errors.Is(wrapped, ErrTOTPRequired) {
		t.Fatal("errors.Is must reach ErrTOTPRequired through fmt.Errorf %w")
	}

	doubled := fmt.Errorf("outer: %w", wrapped)
	if !errors.Is(doubled, ErrTOTPRequired) {
		t.Fatal("errors.Is must reach ErrTOTPRequired through two layers of wrapping")
	}
}

func TestErrTOTPRequiredDistinct(t *testing.T) {
	other := errors.New("2FA required but no TOTP provided") // same text, different value
	if errors.Is(other, ErrTOTPRequired) {
		t.Fatal("errors.Is must be value-equal, not text-equal")
	}
}
