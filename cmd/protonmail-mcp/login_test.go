package main

import (
	"bufio"
	"bytes"
	"strings"
	"testing"
)

func TestPromptThenPasswordSharesReader(t *testing.T) {
	stdin := strings.NewReader("user@example.test\nhunter2\n")
	reader := bufio.NewReader(stdin)
	out := &bytes.Buffer{}

	got, err := promptReader(out, reader, "Proton email: ")
	if err != nil {
		t.Fatalf("promptReader: %v", err)
	}
	if got != "user@example.test" {
		t.Fatalf("email = %q, want %q", got, "user@example.test")
	}

	got, err = readPassword(out, stdin, reader)
	if err != nil {
		t.Fatalf("readPassword: %v", err)
	}
	if got != "hunter2" {
		t.Fatalf("password = %q, want %q", got, "hunter2")
	}
}

func TestPasswordPreservesInternalWhitespace(t *testing.T) {
	stdin := strings.NewReader("user@example.test\n  pa ss \n")
	reader := bufio.NewReader(stdin)
	out := &bytes.Buffer{}

	if _, err := promptReader(out, reader, "email: "); err != nil {
		t.Fatal(err)
	}
	got, err := readPassword(out, stdin, reader)
	if err != nil {
		t.Fatalf("readPassword: %v", err)
	}
	if got != "  pa ss " {
		t.Fatalf("password = %q, want %q (leading/trailing/internal spaces preserved)", got, "  pa ss ")
	}
}

func TestPromptCRLF(t *testing.T) {
	stdin := strings.NewReader("user@example.test\r\nsecret\r\n")
	reader := bufio.NewReader(stdin)
	out := &bytes.Buffer{}

	email, err := promptReader(out, reader, "email: ")
	if err != nil {
		t.Fatal(err)
	}
	if email != "user@example.test" {
		t.Fatalf("email = %q", email)
	}
	pass, err := readPassword(out, stdin, reader)
	if err != nil {
		t.Fatal(err)
	}
	if pass != "secret" {
		t.Fatalf("password = %q", pass)
	}
}

func TestPromptEOF(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader(""))
	out := &bytes.Buffer{}
	if _, err := promptReader(out, reader, "x: "); err == nil {
		t.Fatal("expected EOF error, got nil")
	}
}

func TestPromptAndPasswordNoTrailingNewline(t *testing.T) {
	stdin := strings.NewReader("user@example.test\nhunter2")
	reader := bufio.NewReader(stdin)
	out := &bytes.Buffer{}

	got, err := promptReader(out, reader, "email: ")
	if err != nil {
		t.Fatalf("promptReader: %v", err)
	}
	if got != "user@example.test" {
		t.Fatalf("email = %q", got)
	}
	pass, err := readPassword(out, stdin, reader)
	if err != nil {
		t.Fatalf("readPassword (no trailing newline): %v", err)
	}
	if pass != "hunter2" {
		t.Fatalf("password = %q, want %q", pass, "hunter2")
	}
}

func TestEmptyPasswordLine(t *testing.T) {
	stdin := strings.NewReader("user@example.test\n\n")
	reader := bufio.NewReader(stdin)
	out := &bytes.Buffer{}

	if _, err := promptReader(out, reader, "email: "); err != nil {
		t.Fatal(err)
	}
	got, err := readPassword(out, stdin, reader)
	if err != nil {
		t.Fatalf("readPassword: %v", err)
	}
	if got != "" {
		t.Fatalf("password = %q, want empty string", got)
	}
}

func TestPasswordUTF8(t *testing.T) {
	want := "pässwörd_密码_🔑"
	stdin := strings.NewReader("user@example.test\n" + want + "\n")
	reader := bufio.NewReader(stdin)
	out := &bytes.Buffer{}

	if _, err := promptReader(out, reader, "email: "); err != nil {
		t.Fatal(err)
	}
	got, err := readPassword(out, stdin, reader)
	if err != nil {
		t.Fatalf("readPassword: %v", err)
	}
	if got != want {
		t.Fatalf("password = %q, want %q", got, want)
	}
}

func TestPasswordSingleCRLFStripped(t *testing.T) {
	// strip exactly one trailing \n then one trailing \r; "foo\r\r\n" → "foo\r".
	stdin := strings.NewReader("user@example.test\nfoo\r\r\n")
	reader := bufio.NewReader(stdin)
	out := &bytes.Buffer{}

	if _, err := promptReader(out, reader, "email: "); err != nil {
		t.Fatal(err)
	}
	got, err := readPassword(out, stdin, reader)
	if err != nil {
		t.Fatal(err)
	}
	if got != "foo\r" {
		t.Fatalf("password = %q, want %q (only one CRLF stripped)", got, "foo\r")
	}
}
