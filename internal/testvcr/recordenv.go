package testvcr

import (
	"fmt"
	"os"
	"strings"
)

// RecordEmail returns RECORD_EMAIL normalized the same way the scrubber matches
// it. The scrubber rewrites the trimmed value out of recorded cassettes, so the
// login path MUST authenticate with the trimmed value too — otherwise a
// stray-whitespace RECORD_EMAIL logs in under one address while the scrubber
// rewrites a different one, leaking the real address into the recording.
func RecordEmail() string {
	return strings.TrimSpace(os.Getenv("RECORD_EMAIL"))
}

// RecordCredentials reads the RECORD_* login env shared by every recording
// scenario through one boundary: RECORD_EMAIL is normalized via RecordEmail so
// it can never drift from the scrubber, and both values are validated
// empty-after-trim so a shell-truncated secret fails fast locally with an
// actionable message instead of an opaque upstream WrongPassword round-trip.
//
// The password is returned raw (only its trimmed form is checked for
// emptiness): trimming a real password would silently mangle it.
func RecordCredentials() (email, password string, err error) {
	email = RecordEmail()
	password = os.Getenv("RECORD_PASSWORD")
	if email == "" {
		return "", "", fmt.Errorf("RECORD_EMAIL is empty after trimming whitespace")
	}
	if strings.TrimSpace(password) == "" {
		return "", "", fmt.Errorf(
			"RECORD_PASSWORD is empty or whitespace-only — the shell may have truncated it; " +
				"single-quote the value or set it with `read -rs RECORD_PASSWORD`")
	}
	return email, password, nil
}
