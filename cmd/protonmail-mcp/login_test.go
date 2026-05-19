package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

// TestPromptAndPassword exercises promptReader + readPassword across the
// stdin shapes that real users feed in: LF / CRLF, missing trailing newline,
// empty password, internal whitespace, multi-byte UTF-8, and EOF.
func TestPromptAndPassword(t *testing.T) {
	tests := []struct {
		name         string
		stdin        string
		wantEmail    string
		wantPass     string
		wantEmailErr bool
	}{
		{
			name:      "lf-line-endings",
			stdin:     "user@example.test\nhunter2\n",
			wantEmail: "user@example.test", wantPass: "hunter2",
		},
		{
			name:      "preserves-internal-whitespace",
			stdin:     "user@example.test\n  pa ss \n",
			wantEmail: "user@example.test", wantPass: "  pa ss ",
		},
		{
			name:      "crlf-line-endings",
			stdin:     "user@example.test\r\nsecret\r\n",
			wantEmail: "user@example.test", wantPass: "secret",
		},
		{
			name:      "no-trailing-newline",
			stdin:     "user@example.test\nhunter2",
			wantEmail: "user@example.test", wantPass: "hunter2",
		},
		{
			name:      "empty-password-line",
			stdin:     "user@example.test\n\n",
			wantEmail: "user@example.test", wantPass: "",
		},
		{
			name:      "utf8-password",
			stdin:     "user@example.test\npässwörd_密码_🔑\n",
			wantEmail: "user@example.test", wantPass: "pässwörd_密码_🔑",
		},
		{
			// strip exactly one trailing \n then one trailing \r;
			// "foo\r\r\n" → "foo\r" (only a single CRLF removed).
			name:      "single-crlf-stripped",
			stdin:     "user@example.test\nfoo\r\r\n",
			wantEmail: "user@example.test", wantPass: "foo\r",
		},
		{
			name:         "eof-on-prompt",
			stdin:        "",
			wantEmailErr: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdin := strings.NewReader(tc.stdin)
			reader := bufio.NewReader(stdin)
			out := &bytes.Buffer{}

			email, err := promptReader(out, reader, "Proton email: ")
			if tc.wantEmailErr {
				if err == nil {
					t.Fatal("promptReader: want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("promptReader: %v", err)
			}
			if email != tc.wantEmail {
				t.Fatalf("email = %q, want %q", email, tc.wantEmail)
			}

			pass, err := readPassword(out, stdin, reader)
			if err != nil {
				t.Fatalf("readPassword: %v", err)
			}
			if pass != tc.wantPass {
				t.Fatalf("password = %q, want %q", pass, tc.wantPass)
			}
		})
	}
}
