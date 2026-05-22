package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestIsAllDigits(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"123456", true},
		{"0", true},
		{"abc", false},
		{"123a", false},
		{"a123", false},
		{"12 34", false},
		{"-123", false},
		{"123.4", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := isAllDigits(tc.in); got != tc.want {
				t.Fatalf("isAllDigits(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestRunLoginEOFOnEmail exercises the runLogin entry point's email-prompt
// EOF branch, which surfaces the "unexpected EOF" error from promptReader.
// No cassette needed because the failure happens before any network call.
func TestRunLoginEOFOnEmail(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := runLogin(context.Background(),
		"https://mail.proton.me/api",
		nil,
		strings.NewReader(""),
		stdout, stderr)
	if err == nil {
		t.Fatal("want error from empty stdin, got nil")
	}
	if !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunLoginEOFOnPassword exercises the password-prompt EOF branch.
func TestRunLoginEOFOnPassword(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	err := runLogin(context.Background(),
		"https://mail.proton.me/api",
		nil,
		strings.NewReader("user@example.test\n"),
		stdout, stderr)
	if err == nil {
		t.Fatal("want error from EOF on password, got nil")
	}
	if !strings.Contains(err.Error(), "EOF") {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestRunLoginDefaultAPIURL covers the apiURL == "" branch which substitutes
// the default Proton API URL.
func TestRunLoginDefaultAPIURL(t *testing.T) {
	stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
	// Empty stdin forces the email-prompt EOF path; the apiURL default is
	// applied before the prompt fires, so coverage reaches that branch.
	_ = runLogin(context.Background(),
		"",
		nil,
		strings.NewReader(""),
		stdout, stderr)
}
