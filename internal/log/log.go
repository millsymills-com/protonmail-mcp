// Package log provides a slog logger with automatic redaction of credential-bearing
// fields. Field names containing any of the substrings in sensitiveSubstrings (case
// insensitive) have their values replaced with "<redacted>".
package log

import (
	"context"
	"io"
	"log/slog"
	"strings"
)

var sensitiveSubstrings = []string{
	"password",
	"passphrase",
	"token",
	"secret",
	"totp",
	"key",
}

// New returns a JSON slog logger that writes to w (use os.Stderr in production).
// Redaction is applied to all attribute names regardless of nesting.
func New(level slog.Level, w io.Writer) *slog.Logger {
	base := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level:     level,
		AddSource: false,
	})
	return slog.New(&redactingHandler{inner: base})
}

type redactingHandler struct {
	inner slog.Handler
}

func (h *redactingHandler) Enabled(ctx context.Context, l slog.Level) bool {
	return h.inner.Enabled(ctx, l)
}

func (h *redactingHandler) Handle(ctx context.Context, r slog.Record) error {
	clone := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(a slog.Attr) bool {
		clone.AddAttrs(redactAttr(a))
		return true
	})
	return h.inner.Handle(ctx, clone)
}

func (h *redactingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	red := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		red[i] = redactAttr(a)
	}
	return &redactingHandler{inner: h.inner.WithAttrs(red)}
}

func (h *redactingHandler) WithGroup(name string) slog.Handler {
	return &redactingHandler{inner: h.inner.WithGroup(name)}
}

func redactAttr(a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindGroup {
		gs := a.Value.Group()
		out := make([]slog.Attr, len(gs))
		for i, g := range gs {
			out[i] = redactAttr(g)
		}
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(out...)}
	}
	if isSensitive(a.Key) {
		return slog.String(a.Key, "<redacted>")
	}
	return a
}

func isSensitive(name string) bool {
	lower := strings.ToLower(name)
	for _, s := range sensitiveSubstrings {
		if strings.Contains(lower, s) {
			return true
		}
	}
	return false
}
