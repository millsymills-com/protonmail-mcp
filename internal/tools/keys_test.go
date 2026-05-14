package tools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ProtonMail/gopenpgp/v2/crypto"

	"github.com/millsmillsymills/protonmail-mcp/internal/testharness"
)

func TestListAddressKeysHappyCassette(t *testing.T) {
	h := testharness.BootWithCassette(t, "list_address_keys_happy")
	defer h.Close()
	ctx := context.Background()

	addrsOut, err := h.Call(ctx, "proton_list_addresses", map[string]any{})
	if err != nil {
		t.Fatalf("list_addresses: %v", err)
	}
	addrs, ok := addrsOut["addresses"].([]any)
	if !ok || len(addrs) == 0 {
		t.Fatalf("expected at least one address, got %#v", addrsOut)
	}
	first, ok := addrs[0].(map[string]any)
	if !ok {
		t.Fatalf("address[0] not an object: %#v", addrs[0])
	}
	id, ok := first["id"].(string)
	if !ok || id == "" {
		t.Fatalf("address[0].id missing or empty: %#v", first)
	}

	out, err := h.Call(ctx, "proton_list_address_keys", map[string]any{"address_id": id})
	if err != nil {
		t.Fatalf("list_address_keys: %v", err)
	}
	if _, ok := out["keys"]; !ok {
		t.Fatalf("envelope missing %q", "keys")
	}

	// Cassette response must not contain private key material.
	b, _ := json.Marshal(out)
	if strings.Contains(string(b), "BEGIN PGP PRIVATE KEY BLOCK") {
		t.Fatal("response contains private key material")
	}
}

// TestListAddressKeys_HappyPath_GopenpgpRegression locks the fork->upstream
// swap (gopenpgp/v2 v2.10.0-proton -> v2.10.0, go-crypto v1.4.1-proton ->
// v1.4.1). The tool's silent-failure design (internal/tools/keys.go) means
// any behavioural drift in crypto.NewKey / GetFingerprint /
// GetArmoredPublicKey would land as empty fingerprint and public_key_armored
// fields with no error.
//
// The dev server in testharness generates a real PGP keypair when it creates
// the harness's primary user, so a single end-to-end call exercises the full
// path: API JSON -> Key.UnmarshalJSON (NewKeyFromArmored) -> binary
// PrivateKey -> crypto.NewKey -> GetFingerprint / GetArmoredPublicKey. The
// armored output round-trips through crypto.NewKeyFromArmored to the same
// fingerprint to catch armoring-format regressions.
func TestListAddressKeys_HappyPath_GopenpgpRegression(t *testing.T) {
	h := testharness.BootDevServer(t, "user@example.test", "hunter2")
	ctx := context.Background()

	addrsOut, err := h.Call(ctx, "proton_list_addresses", map[string]any{})
	if err != nil {
		t.Fatalf("proton_list_addresses: %v", err)
	}
	addrs, ok := addrsOut["addresses"].([]any)
	if !ok || len(addrs) == 0 {
		t.Fatalf("expected at least one address, got %#v", addrsOut)
	}
	first, ok := addrs[0].(map[string]any)
	if !ok {
		t.Fatalf("address[0] not an object: %#v", addrs[0])
	}
	addrID, ok := first["id"].(string)
	if !ok {
		t.Fatalf("address[0].id not a string: %#v", first["id"])
	}
	if addrID == "" {
		t.Fatalf("address[0].id empty: %#v", first)
	}

	keysOut, err := h.Call(ctx, "proton_list_address_keys", map[string]any{"address_id": addrID})
	if err != nil {
		t.Fatalf("proton_list_address_keys: %v", err)
	}
	keys, ok := keysOut["keys"].([]any)
	if !ok || len(keys) == 0 {
		t.Fatalf("expected at least one key, got %#v", keysOut)
	}
	k0, ok := keys[0].(map[string]any)
	if !ok {
		t.Fatalf("key[0] not an object: %#v", keys[0])
	}

	fp, ok := k0["fingerprint"].(string)
	if !ok {
		t.Fatalf("fingerprint not a string: %#v", k0["fingerprint"])
	}
	if fp == "" {
		t.Fatalf("fingerprint empty - regression in crypto.NewKey/GetFingerprint")
	}
	armored, ok := k0["public_key_armored"].(string)
	if !ok {
		t.Fatalf("public_key_armored not a string: %#v", k0["public_key_armored"])
	}
	if armored == "" {
		t.Fatalf("public_key_armored empty - regression in GetArmoredPublicKey")
	}

	pk, err := crypto.NewKeyFromArmored(armored)
	if err != nil {
		t.Fatalf("round-trip NewKeyFromArmored on tool output: %v", err)
	}
	if got := pk.GetFingerprint(); got != fp {
		t.Fatalf("fingerprint round-trip mismatch: armored=%q tool=%q", got, fp)
	}
	if pk.IsPrivate() {
		t.Fatal("public_key_armored contained private key material - regression in GetArmoredPublicKey")
	}
}

// TestListAddressKeys_SilentFailureContract verifies the gopenpgp contract
// that internal/tools/keys.go's best-effort fingerprint extraction relies
// on: crypto.NewKey on non-PGP bytes must return (nil, error) or (nil, nil)
// so the inline guard
//
//	if pk, perr := crypto.NewKey(k.PrivateKey); perr == nil && pk != nil { ... }
//
// reliably skips fingerprint/armoring without panicking or short-circuiting
// the rest of the response.
//
// The contract is asserted directly rather than through the harness because
// go-proton-api's Key.UnmarshalJSON calls crypto.NewKeyFromArmored on the
// wire PrivateKey field and surfaces parse errors before the tool runs - so
// an interceptor cannot deliver garbage bytes to keys.go through normal API
// flow. This test instead locks the upstream behaviour the silent-failure
// path depends on.
func TestListAddressKeys_SilentFailureContract(t *testing.T) {
	cases := []struct {
		name string
		in   []byte
	}{
		{"ascii_garbage", []byte("not a pgp key")},
		{"nil_slice", nil},
		{"empty_slice", []byte{}},
		{"random_bytes", []byte{0x00, 0x01, 0x02, 0x03}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pk, err := crypto.NewKey(tc.in)
			if err == nil && pk != nil {
				t.Fatalf("crypto.NewKey(%x): want error or nil key, got pk=%v", tc.in, pk)
			}
		})
	}
}
