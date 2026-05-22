//go:build recording

package scenarios

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"

	srp "github.com/ProtonMail/go-srp"
)

// loginFixturePassword is the password the consumer test types into stdin
// (see cmd/protonmail-mcp/run_test.go::TestLoginNo2FA). The recorder must
// drive its synthetic SRP exchange with the same value so the cassette
// captures a body the consumer can produce at replay.
const loginFixturePassword = "hunter2"

// loginFixtureEmail matches the consumer test's stdin.
const loginFixtureEmail = "user@example.test"

// loginFixtureModulusClearSign is the Proton-signed test modulus from
// go-srp's own test suite. It carries a real signature from Proton's auth
// signing key, so go-srp's NewAuth accepts it. The modulus value is global
// (one prime for the whole Proton ecosystem) and not PII.
const loginFixtureModulusClearSign = `-----BEGIN PGP SIGNED MESSAGE-----
Hash: SHA256

W2z5HBi8RvsfYzZTS7qBaUxxPhsfHJFZpu3Kd6s1JafNrCCH9rfvPLrfuqocxWPgWDH2R8neK7PkNvjxto9TStuY5z7jAzWRvFWN9cQhAKkdWgy0JY6ywVn22+HFpF4cYesHrqFIKUPDMSSIlWjBVmEJZ/MusD44ZT29xcPrOqeZvwtCffKtGAIjLYPZIEbZKnDM1Dm3q2K/xS5h+xdhjnndhsrkwm9U9oyA2wxzSXFL+pdfj2fOdRwuR5nW0J2NFrq3kJjkRmpO/Genq1UW+TEknIWAb6VzJJJA244K/H8cnSx2+nSNZO3bbo6Ys228ruV9A8m6DhxmS+bihN3ttQ==
-----BEGIN PGP SIGNATURE-----
Version: ProtonMail
Comment: https://protonmail.com

wl4EARYIABAFAlwB1j0JEDUFhcTpUY8mAAD8CgEAnsFnF4cF0uSHKkXa1GIa
GO86yMV4zDZEZcDSJo0fgr8A/AlupGN9EdHlsrZLmTA1vhIx+rOgxdEff28N
kvNM7qIK
=q6vu
-----END PGP SIGNATURE-----`

// loginFixtureSalt is a fixed 16-byte salt that matches the chosen password
// in the synthetic SRP exchange. Any 16-byte value works; this one is
// stable so the recorded cassette is reproducible across re-records.
var loginFixtureSalt = []byte{
	0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, 0xef,
	0x10, 0x32, 0x54, 0x76, 0x98, 0xba, 0xdc, 0xfe,
}

// loginFixture2FACode is the 6-digit code the 2FA-variant consumer test
// types after the post-SRP prompt. The fake server compares the submitted
// code against this constant.
const loginFixture2FACode = "123456"

// newFakeProtonAuthServer spins up an httptest.Server that speaks just enough
// of the Proton auth API (/auth/v4/info + /auth/v4) to satisfy a single
// password-only login with the fixture credentials above. SRP math is run
// server-side via go-srp so the ClientProof the proton library submits is
// verified for real and the returned ServerProof matches.
//
// The server intentionally does NOT enforce TwoFA, captcha, or any other
// branch — its sole purpose is to feed the recorder enough wire-level
// truth to fill the login_no_2fa cassette with a structurally valid SRP
// exchange and synthetic auth tokens.
func newFakeProtonAuthServer() (*httptest.Server, error) {
	return newFakeProtonAuthServerWithTwoFA(false)
}

// newFakeProtonAuthServerTwoFA is the 2FA-enabled variant: the post-SRP
// /auth/v4 response signals TwoFA.Enabled = HasTOTP, gating success behind
// a /auth/v4/2fa POST that verifies the fixture code.
func newFakeProtonAuthServerTwoFA() (*httptest.Server, error) {
	return newFakeProtonAuthServerWithTwoFA(true)
}

func newFakeProtonAuthServerWithTwoFA(twoFA bool) (*httptest.Server, error) {
	// Pre-compute the SRP verifier for the fixture password against the
	// signed modulus. This is the server-side persistent secret that
	// would normally live in Proton's user DB.
	auth, err := srp.NewAuthForVerifier(
		[]byte(loginFixturePassword), loginFixtureModulusClearSign, loginFixtureSalt)
	if err != nil {
		return nil, fmt.Errorf("fake-proton: build auth for verifier: %w", err)
	}
	const bitLength = 2048
	verifier, err := auth.GenerateVerifier(bitLength)
	if err != nil {
		return nil, fmt.Errorf("fake-proton: generate verifier: %w", err)
	}

	srpSrv, err := srp.NewServerFromSigned(loginFixtureModulusClearSign, verifier, bitLength)
	if err != nil {
		return nil, fmt.Errorf("fake-proton: NewServerFromSigned: %w", err)
	}
	serverEphemeral, err := srpSrv.GenerateChallenge()
	if err != nil {
		return nil, fmt.Errorf("fake-proton: GenerateChallenge: %w", err)
	}

	saltB64 := base64.StdEncoding.EncodeToString(loginFixtureSalt)
	serverEphemeralB64 := base64.StdEncoding.EncodeToString(serverEphemeral)
	const srpSession = "REDACTED_SRPSESSION_1"

	// Routes are mounted under /api/ to mirror Proton's production path
	// shape. The consumer test boots with PROTONMAIL_MCP_API_URL pointing at
	// https://mail.proton.me/api; matching at replay time depends on the
	// recorded path including the /api segment.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/v4/info", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"Code":            1000,
			"Modulus":         loginFixtureModulusClearSign,
			"ServerEphemeral": serverEphemeralB64,
			"Version":         4,
			"Salt":            saltB64,
			"SRPSession":      srpSession,
			"2FA":             map[string]any{"Enabled": 0},
		})
	})
	mux.HandleFunc("/api/auth/v4", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ClientEphemeral string
			ClientProof     string
			SRPSession      string
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"Code": 2001, "Error": "read body: " + err.Error(),
			})
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"Code": 2001, "Error": "decode body: " + err.Error(),
			})
			return
		}
		clientEph, _ := base64.StdEncoding.DecodeString(req.ClientEphemeral)
		clientProof, _ := base64.StdEncoding.DecodeString(req.ClientProof)
		serverProof, err := srpSrv.VerifyProofs(clientEph, clientProof)
		if err != nil {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"Code": 8002, "Error": "Incorrect login credentials.",
			})
			return
		}
		twoFAEnabled := 0
		if twoFA {
			twoFAEnabled = 1 // proton.HasTOTP
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"Code":         1000,
			"UserID":       "REDACTED_USERID_1",
			"UID":          "REDACTED_UID_1",
			"AccessToken":  "REDACTED_ACCESSTOKEN_1",
			"RefreshToken": "REDACTED_REFRESHTOKEN_1",
			"ServerProof":  base64.StdEncoding.EncodeToString(serverProof),
			"Scope":        "full self parent user loggedin paid nondelinquent mail verified settings",
			"2FA":          map[string]any{"Enabled": twoFAEnabled},
			"PasswordMode": 1,
		})
	})
	// 2FA submission endpoint. Only relevant when twoFA=true; we still
	// register it so an accidental hit in no-2FA mode fails predictably.
	mux.HandleFunc("/api/auth/v4/2fa", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			TwoFactorCode string
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"Code": 2001, "Error": "read body: " + err.Error(),
			})
			return
		}
		if err := json.Unmarshal(body, &req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"Code": 2001, "Error": "decode body: " + err.Error(),
			})
			return
		}
		if !twoFA || req.TwoFactorCode != loginFixture2FACode {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"Code": 8002, "Error": "Incorrect 2FA code.",
			})
			return
		}
		// Post-2FA Proton rotates the refresh token. Return a fresh one so
		// the session library exercises the post-2FA capture path (see
		// session.Login's AddPostRequestHook).
		writeJSON(w, http.StatusOK, map[string]any{
			"Code":         1000,
			"UID":          "REDACTED_UID_1",
			"AccessToken":  "REDACTED_ACCESSTOKEN_2",
			"RefreshToken": "REDACTED_REFRESHTOKEN_2",
			"Scope":        "full self parent user loggedin paid nondelinquent mail verified settings",
		})
	})
	// The proton library also sometimes hits /auth/v4/sessions (logout) and
	// /auth/v4/refresh. Provide minimal stubs in case the recorder triggers
	// them — Login itself only calls /auth/v4/info + /auth/v4 + /auth/v4/2fa.
	mux.HandleFunc("/api/auth/v4/sessions", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"Code": 1000})
	})

	return httptest.NewServer(loginFixtureSecHeaders(mux)), nil
}

// loginFixtureSecHeaders adds the headers the proton library expects to see
// from a real Proton response (Content-Type, Date) so the resty handlers
// don't choke on missing fields during recording.
func loginFixtureSecHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Date", "Mon, 01 Jan 2001 00:00:00 GMT")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

