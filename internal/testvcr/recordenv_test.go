package testvcr

import "testing"

func TestRecordEmailTrimsWhitespace(t *testing.T) {
	t.Setenv("RECORD_EMAIL", "  me@protonmail.com\n")
	if got := RecordEmail(); got != "me@protonmail.com" {
		t.Fatalf("RecordEmail() = %q, want trimmed", got)
	}
}

func TestRecordCredentialsTrimsEmailToMatchScrubber(t *testing.T) {
	t.Setenv("RECORD_EMAIL", " me@protonmail.com ")
	t.Setenv("RECORD_PASSWORD", "hunter2")
	email, password, err := RecordCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if email != "me@protonmail.com" {
		t.Fatalf("email = %q, want trimmed to match scrubber boundary", email)
	}
	if password != "hunter2" {
		t.Fatalf("password = %q, want returned raw", password)
	}
}

func TestRecordCredentialsKeepsPasswordRaw(t *testing.T) {
	t.Setenv("RECORD_EMAIL", "me@protonmail.com")
	t.Setenv("RECORD_PASSWORD", "  pa ss  ")
	_, password, err := RecordCredentials()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if password != "  pa ss  " {
		t.Fatalf("password = %q, want untouched (trimming mangles real passwords)", password)
	}
}

func TestRecordCredentialsFailsFastOnEmptyEmail(t *testing.T) {
	t.Setenv("RECORD_EMAIL", "   ")
	t.Setenv("RECORD_PASSWORD", "hunter2")
	email, password, err := RecordCredentials()
	if err == nil {
		t.Fatal("want error for whitespace-only RECORD_EMAIL")
	}
	if email != "" || password != "" {
		t.Fatalf("credentials returned alongside error: email=%q password set=%v", email, password != "")
	}
}

func TestRecordCredentialsFailsFastOnWhitespacePassword(t *testing.T) {
	t.Setenv("RECORD_EMAIL", "me@protonmail.com")
	t.Setenv("RECORD_PASSWORD", "   ")
	email, password, err := RecordCredentials()
	if err == nil {
		t.Fatal("want error for whitespace-only RECORD_PASSWORD (shell truncation)")
	}
	if email != "" || password != "" {
		t.Fatalf("credentials returned alongside error: email=%q password set=%v", email, password != "")
	}
}
