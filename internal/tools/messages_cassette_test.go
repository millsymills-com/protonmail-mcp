package tools_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/keyring"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

// keyringErrSession wraps a real session.Service but forces Keyrings to fail,
// exercising the proton_get_message include_body unlock-error branch.
type keyringErrSession struct {
	session.Service
	err error
}

func (s keyringErrSession) Keyrings(context.Context) (*keyring.Keyrings, error) {
	return nil, s.err
}

// decryptErrSession returns an empty keyring so DecryptBody fails to find a
// keyring for the message's address, exercising the decrypt-error branch.
type decryptErrSession struct {
	session.Service
}

func (s decryptErrSession) Keyrings(context.Context) (*keyring.Keyrings, error) {
	return &keyring.Keyrings{}, nil
}

func TestSearchMessagesHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "search_messages_happy")
	defer h.Close()
	out, err := h.Call(context.Background(), "proton_search_messages", map[string]any{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if _, ok := out["messages"]; !ok {
		t.Fatalf("envelope missing %q", "messages")
	}
}

// TestGetMessageHappyCassette is currently skipped: the recorded cassette
// was removed because get_message returns full inbox content (RFC2822
// headers, sender names, DKIM signatures, Subject, X-Pm-Spam blob, etc.)
// and the testvcr scrubber only handles structured Proton-API JSON, not
// arbitrary email content. Re-record only after a controlled plain-text
// message has been sent to the recording account; see the follow-up issue
// linked from #63.
func TestGetMessageHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "get_message_happy")
	defer h.Close()
	ctx := context.Background()

	searchOut, err := h.Call(ctx, "proton_search_messages", map[string]any{"limit": float64(1)})
	if err != nil {
		t.Fatalf("search_messages: %v", err)
	}
	msgs, ok := searchOut["messages"].([]any)
	if !ok {
		t.Fatalf("messages not a list: %#v", searchOut["messages"])
	}
	// The cassette was recorded against an account with at least one message.
	// Skip rather than fail if the cassette is present but empty (e.g. new test
	// account with no messages).
	if len(msgs) == 0 {
		t.Skip("cassette has no messages; re-record after sending a message to the test account")
	}
	first, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("message[0] not an object: %#v", msgs[0])
	}
	id, ok := first["id"].(string)
	if !ok || id == "" {
		t.Fatalf("message[0].id missing or empty: %#v", first)
	}

	out, err := h.Call(ctx, "proton_get_message", map[string]any{"id": id})
	if err != nil {
		t.Fatalf("get_message: %v", err)
	}
	if _, ok := out["message"]; !ok {
		t.Fatalf("envelope missing %q", "message")
	}
}

// firstMessageID returns the id of the first message the search cassette
// yields, skipping the test when the cassette holds none.
func firstMessageID(t *testing.T, h *testharness.Harness, ctx context.Context) string {
	t.Helper()
	searchOut, err := h.Call(ctx, "proton_search_messages", map[string]any{"limit": float64(1)})
	if err != nil {
		t.Fatalf("search_messages: %v", err)
	}
	msgs, ok := searchOut["messages"].([]any)
	if !ok || len(msgs) == 0 {
		t.Skip("cassette has no messages; re-record after sending a message to the test account")
	}
	first, ok := msgs[0].(map[string]any)
	if !ok {
		t.Fatalf("message[0] not an object: %#v", msgs[0])
	}
	id, ok := first["id"].(string)
	if !ok || id == "" {
		t.Fatalf("message[0].id missing or empty: %#v", first)
	}
	return id
}

// TestGetMessageIncludeBodyErrorPaths covers the two handler error branches the
// happy cassette can't reach: a keyring unlock failure and a body that won't
// decrypt. Both wrap the real cassette-backed session (so the message fetch
// still succeeds) and assert the handler returns the mapped failure result,
// not a transport error.
func TestGetMessageIncludeBodyErrorPaths(t *testing.T) {
	tests := []struct {
		name     string
		wrap     func(session.Service) session.Service
		wantCode string
	}{
		{
			name: "keyring unlock error",
			wrap: func(s session.Service) session.Service {
				return keyringErrSession{Service: s, err: fmt.Errorf("unlock keyrings: %w", proterr.ErrKeyringLocked)}
			},
			wantCode: "proton/keyring_locked",
		},
		{
			name:     "decrypt body error",
			wrap:     func(s session.Service) session.Service { return decryptErrSession{Service: s} },
			wantCode: "proton/body_undecryptable",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := testharness.BootWithCassette(t, "get_message_happy", testharness.WithSessionService(tc.wrap))
			defer h.Close()
			ctx := context.Background()

			id := firstMessageID(t, h, ctx)
			_, err := h.Call(ctx, "proton_get_message", map[string]any{"id": id, "include_body": true})
			if err == nil {
				t.Fatal("expected include_body failure, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantCode) {
				t.Fatalf("error = %q, want code %q", err.Error(), tc.wantCode)
			}
		})
	}
}
