package testvcr_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func TestLintFlagsBearerToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "leaky.yaml")
	if err := os.WriteFile(path, []byte("Authorization: Bearer eyJraWQiOi.ABCDEFGHIJ.signature\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := testvcr.Scan(dir)
	if len(got) == 0 {
		t.Fatal("expected at least one finding")
	}
	if got[0].Rule != "bearer-token" {
		t.Fatalf("rule = %q, want bearer-token", got[0].Rule)
	}
}

func TestLintAllowsPublicPGP(t *testing.T) {
	dir := t.TempDir()
	body := "-----BEGIN PGP PUBLIC KEY BLOCK-----\nstuff\n-----END PGP PUBLIC KEY BLOCK-----\n"
	if err := os.WriteFile(filepath.Join(dir, "public.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := testvcr.Scan(dir)
	for _, f := range got {
		if f.Rule == "pgp" {
			t.Fatalf("public key flagged: %+v", f)
		}
	}
}

func TestLintFlagsProtonPGPArmor(t *testing.T) {
	// pgp-proton fires on any Proton-tagged armor (PRIVATE KEY BLOCK,
	// MESSAGE, SIGNATURE) followed by "Version: ProtonMail". The in-repo
	// gopenpgp fixture uses "Version: GopenPGP" so it must not match.
	cases := []struct {
		name string
		body string
	}{
		{"private-key-block", `"-----BEGIN PGP PRIVATE KEY BLOCK-----\nVersion: ProtonMail\n\nxsBN..."`},
		{"message", `"-----BEGIN PGP MESSAGE-----\nVersion: ProtonMail\n\nwcB..."`},
		{"signature", `"-----BEGIN PGP SIGNATURE-----\nVersion: ProtonMail\n\nwsB..."`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "leaky.yaml"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got := testvcr.Scan(dir)
			found := false
			for _, f := range got {
				if f.Rule == "pgp-proton" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected pgp-proton finding; got %+v", got)
			}
		})
	}
}

func TestLintAllowsFixturePGP(t *testing.T) {
	dir := t.TempDir()
	// The recorder-substituted gopenpgp fixture: BEGIN header followed by
	// a Comment line and Version: GopenPGP. Must not match pgp-proton.
	body := `"-----BEGIN PGP PRIVATE KEY BLOCK-----\nComment: https://gopenpgp.org\nVersion: GopenPGP 2.10.0\n\nxsBN..."`
	if err := os.WriteFile(filepath.Join(dir, "fixture.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, f := range testvcr.Scan(dir) {
		if f.Rule == "pgp-proton" {
			t.Fatalf("unexpected pgp-proton finding on fixture: %+v", f)
		}
	}
}

func TestLintFlagsProtonEmail(t *testing.T) {
	// Proton issues addresses on three TLDs — all three must trip the rule.
	cases := []string{"alice@protonmail.com", "bob@proton.me", "carol@pm.me"}
	for _, body := range cases {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "leaky.yaml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			got := testvcr.Scan(dir)
			if len(got) == 0 || got[0].Rule != "proton-email" {
				t.Fatalf("expected proton-email finding for %s; got %+v", body, got)
			}
		})
	}
}

func TestLintAllowsScrubbedAccessToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.yaml"), []byte(`"AccessToken": "REDACTED_ACCESSTOKEN_1"`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := testvcr.Scan(dir); len(got) != 0 {
		t.Fatalf("expected zero findings on scrubbed cassette, got %+v", got)
	}
}

func TestLintNewRulesFireOnRaw(t *testing.T) {
	cases := []struct {
		rule string
		body string
	}{
		{"uid-raw", `"UID": "abc123xyz"`},
		// Lowercase variant — Proton /auth/v4/refresh returns "Uid" alongside
		// "UID". Both must trip uid-raw via the case-insensitive regex.
		{"uid-raw", `"Uid":"g6htg4va3y2qggaazx4wgmj2r5zjykda"`},
		{"key-salt-raw", `"KeySalt": "somesaltvalue"`},
		{"srp-session-raw", `"SrpSession": "srpdata123"`},
		{"server-proof-raw", `"ServerProof": "proofvalue1"`},
		{"client-proof-raw", `"ClientProof": "proofvalue2"`},
		{"client-ephemeral-raw", `"ClientEphemeral": "ephemdata1"`},
		{"two-factor-code-raw", `"TwoFactorCode": "123456789"`},
		{"recovery-secret-raw", `"RecoverySecret": "g8RbbkMK+secretbytes="`},
		{"fingerprint-raw", `"Fingerprint": "e1a4a657f6898d2204d606e235fac8ab7e0011e6"`},
	}
	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "leaky.yaml"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got := testvcr.Scan(dir)
			found := false
			for _, f := range got {
				if f.Rule == tc.rule {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected %q finding; got %+v", tc.rule, got)
			}
		})
	}
}

func TestLintNewRulesIgnoreScrubbed(t *testing.T) {
	cases := []struct {
		rule string
		body string
	}{
		{"uid-raw", `"UID": "REDACTED_UID_1"`},
		{"key-salt-raw", `"KeySalt": "REDACTED_KEYSALT_1"`},
		{"srp-session-raw", `"SrpSession": "REDACTED_SRPSESSION_1"`},
		{"server-proof-raw", `"ServerProof": "REDACTED_SERVERPROOF_1"`},
		{"client-proof-raw", `"ClientProof": "REDACTED_CLIENTPROOF_1"`},
		{"client-ephemeral-raw", `"ClientEphemeral": "REDACTED_CLIENTEPHEMERAL_1"`},
		{"two-factor-code-raw", `"TwoFactorCode": "REDACTED_TWOFACTORCODE_1"`},
		{"recovery-secret-raw", `"RecoverySecret": "REDACTED_RECOVERYSECRET_1"`},
		{"fingerprint-raw", `"Fingerprint": "REDACTED_FINGERPRINT_1"`},
	}
	for _, tc := range cases {
		t.Run(tc.rule, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "ok.yaml"), []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got := testvcr.Scan(dir)
			for _, f := range got {
				if f.Rule == tc.rule {
					t.Fatalf("rule %q fired on scrubbed value; finding: %+v", tc.rule, f)
				}
			}
		})
	}
}

func TestLintReportsReadError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 000 unreliable on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses permission bits")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "locked.yaml")
	if err := os.WriteFile(path, []byte("anything\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	got := testvcr.Scan(dir)
	found := false
	for _, f := range got {
		if f.Rule == "read-error" && f.Path == path {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected read-error finding for %s; got %+v", path, got)
	}
}

func writeMeta(t *testing.T, dir, filename, meta string) {
	t.Helper()
	p := filepath.Join(dir, filename)
	if err := os.WriteFile(p, []byte(meta), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLintWarnsOnStaleCassette(t *testing.T) {
	dir := t.TempDir()
	staleTime := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	meta := fmt.Sprintf(
		"recorded_at: %s\ngo_proton_api_version: %s\nmcp_version: v0.1.0\nscenario: test\n",
		staleTime, testvcr.GoProtonAPIVersion(),
	)
	writeMeta(t, dir, "stale.yaml.meta.yaml", meta)
	got := testvcr.Scan(dir)
	for _, f := range got {
		if f.Rule == "stale-cassette" {
			return
		}
	}
	t.Fatalf("expected stale-cassette finding; got %+v", got)
}

func TestLintWarnsOnVersionDrift(t *testing.T) {
	dir := t.TempDir()
	meta := fmt.Sprintf(
		"recorded_at: %s\ngo_proton_api_version: v0.0.0-old\nmcp_version: v0.1.0\nscenario: test\n",
		time.Now().UTC().Format(time.RFC3339),
	)
	writeMeta(t, dir, "drift.yaml.meta.yaml", meta)
	got := testvcr.Scan(dir)
	for _, f := range got {
		if f.Rule == "version-drift" {
			return
		}
	}
	t.Fatalf("expected version-drift finding; got %+v", got)
}

func TestStrictExitCode(t *testing.T) {
	dir := t.TempDir()
	// Write a stale sidecar (no leak — no hard error).
	staleTime := time.Now().Add(-100 * 24 * time.Hour).UTC().Format(time.RFC3339)
	meta := fmt.Sprintf(
		"recorded_at: %s\ngo_proton_api_version: %s\nmcp_version: v0.1.0\nscenario: test\n",
		staleTime, testvcr.GoProtonAPIVersion(),
	)
	writeMeta(t, dir, "stale.yaml.meta.yaml", meta)

	run := func(strict bool) int {
		cmd := exec.Command("go", "run", "./cmd/testvcr-lint", dir)
		if strict {
			cmd.Env = append(os.Environ(), "STRICT=1")
		}
		cmd.Dir = filepath.Join("..", "..")
		if err := cmd.Run(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				return exitErr.ExitCode()
			}
		}
		return 0
	}

	if code := run(false); code != 0 {
		t.Fatalf("without STRICT: exit %d, want 0", code)
	}
	if code := run(true); code != 1 {
		t.Fatalf("with STRICT=1: exit %d, want 1", code)
	}
}
