package log_test

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	mcplog "github.com/millsmillsymills/protonmail-mcp/internal/log"
)

func TestRedactsSensitiveFields(t *testing.T) {
	var buf bytes.Buffer
	logger := mcplog.New(slog.LevelDebug, &buf)
	logger.Info("auth attempt",
		"username", "andy@example.com",
		"password", "hunter2",
		"refresh_token", "abc.def",
		"totp", "123456",
		"PassPhrase", "shh",
		"api_key", "k-123",
		"private_key", "armored",
		"safe_field", "ok",
	)

	var rec map[string]any
	if err := json.Unmarshal(stripPrefix(buf.Bytes()), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\noutput: %s", err, buf.String())
	}

	tests := []struct {
		key      string
		redacted bool
	}{
		{"username", false},
		{"password", true},
		{"refresh_token", true},
		{"totp", true},
		{"PassPhrase", true},
		{"api_key", true},
		{"private_key", true},
		{"safe_field", false},
	}
	for _, tc := range tests {
		got, _ := rec[tc.key].(string)
		if tc.redacted && got != "<redacted>" {
			t.Errorf("%s: want <redacted>, got %q", tc.key, got)
		}
		if !tc.redacted && got == "<redacted>" {
			t.Errorf("%s: was unexpectedly redacted", tc.key)
		}
	}
}

// strips the slog JSON record prefix if present (no-op for pure-JSON handler)
func stripPrefix(b []byte) []byte {
	if i := bytes.IndexByte(b, '{'); i > 0 {
		return b[i:]
	}
	return bytes.TrimSpace(b)
}

func TestWithAttrsRedacts(t *testing.T) {
	var buf bytes.Buffer
	logger := mcplog.New(slog.LevelDebug, &buf)
	child := logger.With("access_token", "leak", "user", "andy")
	child.Info("login")

	var rec map[string]any
	if err := json.Unmarshal(stripPrefix(buf.Bytes()), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\noutput: %s", err, buf.String())
	}
	if got, _ := rec["access_token"].(string); got != "<redacted>" {
		t.Errorf("access_token: want <redacted>, got %q", got)
	}
	if got, _ := rec["user"].(string); got != "andy" {
		t.Errorf("user: want \"andy\", got %q", got)
	}
}

func TestWithGroupRedacts(t *testing.T) {
	var buf bytes.Buffer
	logger := mcplog.New(slog.LevelDebug, &buf)
	child := logger.WithGroup("auth")
	child.Info("login", "refresh_token", "leak", "user", "andy")

	var rec map[string]any
	if err := json.Unmarshal(stripPrefix(buf.Bytes()), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\noutput: %s", err, buf.String())
	}
	auth, ok := rec["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth group missing: %#v", rec)
	}
	if got, _ := auth["refresh_token"].(string); got != "<redacted>" {
		t.Errorf("auth.refresh_token: want <redacted>, got %q", got)
	}
	if got, _ := auth["user"].(string); got != "andy" {
		t.Errorf("auth.user: want \"andy\", got %q", got)
	}
}

func TestRedactsNestedGroups(t *testing.T) {
	var buf bytes.Buffer
	logger := mcplog.New(slog.LevelDebug, &buf)
	logger.Info("nested",
		slog.Group("auth", "access_token", "leak", "user", "andy"),
	)

	var rec map[string]any
	if err := json.Unmarshal(stripPrefix(buf.Bytes()), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\noutput: %s", err, buf.String())
	}
	auth, ok := rec["auth"].(map[string]any)
	if !ok {
		t.Fatalf("auth group missing or not an object: %#v", rec["auth"])
	}
	if got, _ := auth["access_token"].(string); got != "<redacted>" {
		t.Errorf("auth.access_token: want <redacted>, got %q", got)
	}
	if got, _ := auth["user"].(string); got != "andy" {
		t.Errorf("auth.user: want \"andy\", got %q", got)
	}
}

func TestRedactsDoublyNestedGroups(t *testing.T) {
	var buf bytes.Buffer
	logger := mcplog.New(slog.LevelDebug, &buf)
	logger.Info("doubly nested",
		slog.Group("outer",
			slog.Group("inner", "secret", "leak", "ok", "fine"),
		),
	)

	var rec map[string]any
	if err := json.Unmarshal(stripPrefix(buf.Bytes()), &rec); err != nil {
		t.Fatalf("not valid JSON: %v\noutput: %s", err, buf.String())
	}
	outer, _ := rec["outer"].(map[string]any)
	inner, _ := outer["inner"].(map[string]any)
	if got, _ := inner["secret"].(string); got != "<redacted>" {
		t.Errorf("outer.inner.secret: want <redacted>, got %q", got)
	}
	if got, _ := inner["ok"].(string); got != "fine" {
		t.Errorf("outer.inner.ok: want \"fine\", got %q", got)
	}
}
