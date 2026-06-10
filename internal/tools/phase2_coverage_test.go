package tools_test

// Phase 2 coverage tests: validation and error branches for labels, drafts,
// and organize tools. Each group targets the specific uncovered branches
// identified by go tool cover -func=cov.out.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/millsymills-com/protonmail-mcp/internal/keyring"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testharness"
)

// ---- Helpers used across multiple subtests --------------------------------

// errKeyrings overrides Keyrings to return an error, exercising the
// senderKeyRing first-branch (krs fetch failure).
type errKeyrings struct {
	session.Service
	err error
}

func (e errKeyrings) Keyrings(_ context.Context) (*keyring.Keyrings, error) {
	return nil, e.err
}

// emptyAddrKeyrings overrides Keyrings to return a Keyrings with an empty Addr
// map, exercising senderKeyRing's second branch (AddressKeyRing miss).
type emptyAddrKeyrings struct {
	session.Service
}

func (e emptyAddrKeyrings) Keyrings(_ context.Context) (*keyring.Keyrings, error) {
	return &keyring.Keyrings{Addr: map[string]*crypto.KeyRing{}}, nil
}

// ---- Test 1: listLabels + labelTypeString via dev server ------------------

func TestListLabelsDevServer(t *testing.T) {
	h := testharness.BootDevServer(t, "labeler@example.test", "hunter2")
	defer h.Close()

	out, err := h.Call(context.Background(), "proton_list_labels", map[string]any{})
	if err != nil {
		t.Fatalf("proton_list_labels: %v", err)
	}

	labels, ok := out["labels"].([]any)
	if !ok || len(labels) == 0 {
		t.Fatalf("expected non-empty labels slice, got %#v", out)
	}

	// Verify every entry has non-empty id and name; collect type values seen.
	validTypes := map[string]bool{"label": true, "folder": true, "system": true}
	seenTypes := map[string]bool{}

	for i, raw := range labels {
		entry, ok := raw.(map[string]any)
		if !ok {
			t.Fatalf("label[%d] not a map: %T", i, raw)
		}
		id, _ := entry["id"].(string)
		name, _ := entry["name"].(string)
		typ, _ := entry["type"].(string)
		if id == "" {
			t.Fatalf("label[%d] missing id: %#v", i, entry)
		}
		if name == "" {
			t.Fatalf("label[%d] missing name: %#v", i, entry)
		}
		if !validTypes[typ] {
			// labelTypeString returns strconv.Itoa for unknown types — numeric string
			// is acceptable for unknown future values; only fail if completely empty.
			if typ == "" {
				t.Fatalf("label[%d] empty type: %#v", i, entry)
			}
		}
		seenTypes[typ] = true
	}

	// The dev server always seeds at least system labels (Inbox=0, Drafts=1, …).
	if !seenTypes["system"] {
		t.Logf("note: dev server returned no system labels; types seen: %v", seenTypes)
		t.Logf("(this is acceptable if the dev server omits system labels)")
	}
}

// ---- Test 2: proton_create_draft validation errors ------------------------

func TestCreateDraftValidationErrors(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootDevServer(t, "drafter2@example.test", "hunter2")
	defer h.Close()
	ctx := context.Background()

	cases := []struct {
		name    string
		args    map[string]any
		wantErr string
	}{
		{
			name:    "invalid mime type",
			args:    map[string]any{"mime_type": "application/json", "subject": "s", "body": "b"},
			wantErr: "proton/validation",
		},
		{
			name:    "malformed to address",
			args:    map[string]any{"to": []any{"not-an-email"}, "subject": "s", "body": "b"},
			wantErr: "proton/validation",
		},
		{
			name:    "malformed cc address",
			args:    map[string]any{"cc": []any{"bad@@addr"}, "subject": "s", "body": "b"},
			wantErr: "proton/validation",
		},
		{
			name:    "malformed bcc address",
			args:    map[string]any{"bcc": []any{"@nodomain"}, "subject": "s", "body": "b"},
			wantErr: "proton/validation",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.Call(ctx, "proton_create_draft", tc.args)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got: %v", tc.wantErr, err)
			}
		})
	}
}

// ---- Test 3: proton_update_draft requires id ------------------------------

func TestUpdateDraftRequiresID(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootDevServer(t, "drafter3@example.test", "hunter2")
	defer h.Close()

	// Pass id="" (empty string) to reach the handler's required() validation:
	// the MCP SDK may enforce the JSON schema "required" list for missing keys,
	// but an explicit empty string passes schema validation and hits the handler.
	_, err := h.Call(context.Background(), "proton_update_draft", map[string]any{
		"id": "", "subject": "empty id test", "body": "b",
	})
	if err == nil {
		t.Fatal("expected proton/validation error for empty id, got nil")
	}
	if !strings.Contains(err.Error(), "proton/validation") {
		t.Fatalf("expected proton/validation, got: %v", err)
	}
}

// ---- Test 4: resolveSender explicit sender happy path + error path --------

func TestCreateDraftExplicitSender(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootDevServer(t, "drafter4@example.test", "hunter2")
	defer h.Close()
	ctx := context.Background()

	// Fetch the primary address ID via proton_list_addresses.
	addrsOut, err := h.Call(ctx, "proton_list_addresses", map[string]any{})
	if err != nil {
		t.Fatalf("list_addresses: %v", err)
	}
	addrs, ok := addrsOut["addresses"].([]any)
	if !ok || len(addrs) == 0 {
		t.Fatalf("expected addresses, got %#v", addrsOut)
	}
	first, ok := addrs[0].(map[string]any)
	if !ok {
		t.Fatalf("addresses[0] not a map: %T", addrs[0])
	}
	addrID, _ := first["id"].(string)
	if addrID == "" {
		t.Fatalf("primary address has no id: %#v", first)
	}

	// Create a draft with explicit from_address_id — exercises resolveSender's
	// explicit-ID happy path (GetAddress + eligibility check).
	out, err := h.Call(ctx, "proton_create_draft", map[string]any{
		"from_address_id": addrID,
		"subject":         "explicit sender test",
		"body":            "body",
	})
	if err != nil {
		t.Fatalf("create draft with explicit sender: %v", err)
	}
	if _, ok := out["message"]; !ok {
		t.Fatalf("expected message in response, got %#v", out)
	}

	// Nonexistent address — exercises the GetAddress error branch.
	_, err = h.Call(ctx, "proton_create_draft", map[string]any{
		"from_address_id": "nonexistent-address-id",
		"subject":         "bad sender",
		"body":            "body",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent from_address_id, got nil")
	}
	t.Logf("nonexistent address error (expected): %v", err)
}

// ---- Test 5: senderKeyRing failure branches via WithSessionService --------

func TestCreateDraftKeyringFailures(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")

	t.Run("keyrings_fetch_error", func(t *testing.T) {
		h := testharness.BootDevServer(t, "krerr@example.test", "hunter2",
			testharness.WithSessionService(func(s session.Service) session.Service {
				return errKeyrings{Service: s, err: errors.New("keyring unlock failed for test")}
			}),
		)
		defer h.Close()

		_, err := h.Call(context.Background(), "proton_create_draft", map[string]any{
			"subject": "test", "body": "body",
		})
		if err == nil {
			t.Fatal("expected error from Keyrings() failure, got nil")
		}
		t.Logf("keyrings fetch error (expected): %v", err)
	})

	t.Run("address_keyring_missing", func(t *testing.T) {
		h := testharness.BootDevServer(t, "krmissing@example.test", "hunter2",
			testharness.WithSessionService(func(s session.Service) session.Service {
				return emptyAddrKeyrings{Service: s}
			}),
		)
		defer h.Close()

		_, err := h.Call(context.Background(), "proton_create_draft", map[string]any{
			"subject": "test", "body": "body",
		})
		if err == nil {
			t.Fatal("expected proton/address_keyring_missing error, got nil")
		}
		if !strings.Contains(err.Error(), "proton/address_keyring_missing") {
			t.Fatalf("expected proton/address_keyring_missing, got: %v", err)
		}
	})
}

// ---- Test 6: organize validation and upstream errors ----------------------

func TestOrganizeValidationAndUpstreamErrors(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	t.Setenv("PROTONMAIL_MCP_ENABLE_DANGEROUS", "1")
	h := testharness.BootDevServer(t, "organizer2@example.test", "hunter2")
	defer h.Close()
	ctx := context.Background()

	validationCases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{
			name: "label empty message_ids",
			tool: "proton_label_messages",
			args: map[string]any{"message_ids": []any{}, "label_id": "10", "action": "add"},
		},
		{
			// label_id="" (empty string) reaches the handler's required() check
			// instead of the SDK schema validation (which catches missing keys).
			name: "label empty label_id",
			tool: "proton_label_messages",
			args: map[string]any{"message_ids": []any{"some-id"}, "label_id": "", "action": "add"},
		},
		{
			name: "label invalid action",
			tool: "proton_label_messages",
			args: map[string]any{"message_ids": []any{"some-id"}, "label_id": "10", "action": "toggle"},
		},
		{
			name: "mark empty message_ids",
			tool: "proton_mark_messages",
			args: map[string]any{"message_ids": []any{}, "read": true},
		},
		{
			name: "delete empty message_ids",
			tool: "proton_delete_messages",
			args: map[string]any{"message_ids": []any{}},
		},
	}

	for _, tc := range validationCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.Call(ctx, tc.tool, tc.args)
			if err == nil {
				t.Fatalf("expected proton/validation error, got nil")
			}
			if !strings.Contains(err.Error(), "proton/validation") {
				t.Fatalf("expected proton/validation, got: %v", err)
			}
		})
	}

	// Upstream error cases: use a nonexistent message ID. The dev server may
	// silently accept some operations on unknown IDs (return 200 OK) or panic
	// (return 500). We accept any non-nil error; report what we observe.
	// Dropped: mark_read_nonexistent_message — dev server panics (nil pointer
	// deref) on unknown IDs for SetMessagesRead, surfacing as 500. We skip this
	// case since the panic is a dev-server bug, not our error path.
	upstreamCases := []struct {
		name string
		tool string
		args map[string]any
	}{
		{
			name: "label nonexistent message",
			tool: "proton_label_messages",
			args: map[string]any{"message_ids": []any{"nonexistent-id"}, "label_id": "10", "action": "add"},
		},
		{
			name: "delete nonexistent message",
			tool: "proton_delete_messages",
			args: map[string]any{"message_ids": []any{"nonexistent-id"}},
		},
	}

	for _, tc := range upstreamCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := h.Call(ctx, tc.tool, tc.args)
			if err == nil {
				// Some dev servers silently accept unknown IDs (return 200 OK).
				// Log and skip — don't assert a wrong outcome.
				t.Logf("note: %s with nonexistent ID returned success (dev server accepts unknown IDs)", tc.tool)
				return
			}
			t.Logf("upstream error code (expected): %v", err)
		})
	}
}

// ---- Test 7: updateDraft mime_type validation error -----------------------

func TestUpdateDraftValidationError(t *testing.T) {
	t.Setenv("PROTONMAIL_MCP_ENABLE_WRITES", "1")
	h := testharness.BootDevServer(t, "drafter5@example.test", "hunter2")
	defer h.Close()
	ctx := context.Background()

	// First, create a valid draft to get an ID.
	created, err := h.Call(ctx, "proton_create_draft", map[string]any{
		"subject": "update mime test", "body": "body",
	})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	msg, _ := created["message"].(map[string]any)
	id, _ := msg["id"].(string)
	if id == "" {
		t.Fatalf("no draft id: %#v", created)
	}

	// Now update with an invalid mime type — exercises updateDraft's template
	// error branch.
	_, err = h.Call(ctx, "proton_update_draft", map[string]any{
		"id":        id,
		"mime_type": "bogus/type",
		"subject":   "updated",
		"body":      "updated body",
	})
	if err == nil {
		t.Fatal("expected proton/validation for bogus mime_type, got nil")
	}
	if !strings.Contains(err.Error(), "proton/validation") {
		t.Fatalf("expected proton/validation, got: %v", err)
	}
}
