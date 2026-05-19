package session

import (
	"errors"
	"fmt"
	"testing"
)

func TestErrTOTPRequired(t *testing.T) {
	other := errors.New("2FA required but no TOTP provided") // same text, different value

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"single-wrap", fmt.Errorf("login flow: %w", ErrTOTPRequired), true},
		{"double-wrap", fmt.Errorf("outer: %w", fmt.Errorf("inner: %w", ErrTOTPRequired)), true},
		{"distinct-value-same-text", other, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := errors.Is(tc.err, ErrTOTPRequired); got != tc.want {
				t.Fatalf("errors.Is = %v, want %v", got, tc.want)
			}
		})
	}
}
