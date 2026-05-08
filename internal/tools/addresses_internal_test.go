package tools

import (
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
)

//nolint:revive // cyclomatic: comprehensive field validation requires multiple checks
func TestToAddressDTO_FullPopulation(t *testing.T) {
	in := proton.Address{
		ID:          "addr-1",
		Email:       "a@example.com",
		DisplayName: "Andy",
		Status:      proton.AddressStatusEnabled,
		Order:       1,
		Type:        proton.AddressTypeOriginal,
		Send:        proton.Bool(true),
		Receive:     proton.Bool(true),
		Keys:        []proton.Key{{ID: "k1"}, {ID: "k2"}},
	}
	got := toAddressDTO(in)

	if got.ID != "addr-1" {
		t.Fatalf("ID mismatch: got %q", got.ID)
	}
	if got.Email != "a@example.com" {
		t.Fatalf("Email mismatch: got %q", got.Email)
	}
	if got.DisplayName != "Andy" {
		t.Fatalf("DisplayName mismatch: got %q", got.DisplayName)
	}
	if got.Status != int(proton.AddressStatusEnabled) {
		t.Fatalf("Status mismatch: got %d", got.Status)
	}
	if !got.Send {
		t.Fatalf("Send mismatch: got %v", got.Send)
	}
	if !got.Receive {
		t.Fatalf("Receive mismatch: got %v", got.Receive)
	}
	if len(got.KeyIDs) != 2 {
		t.Fatalf("KeyIDs length mismatch: got %d", len(got.KeyIDs))
	}
	if got.KeyIDs[0] != "k1" {
		t.Fatalf("KeyIDs[0] mismatch: got %q", got.KeyIDs[0])
	}
	if got.KeyIDs[1] != "k2" {
		t.Fatalf("KeyIDs[1] mismatch: got %q", got.KeyIDs[1])
	}
}

func TestToAddressDTO_EmptyKeys(t *testing.T) {
	got := toAddressDTO(proton.Address{ID: "x"})
	if got.KeyIDs == nil {
		t.Fatal("KeyIDs must be non-nil empty slice, not nil (matches make())")
	}
	if len(got.KeyIDs) != 0 {
		t.Fatalf("KeyIDs must be empty, got %v", got.KeyIDs)
	}
}

func TestToAddressDTO_DisabledStatus(t *testing.T) {
	got := toAddressDTO(proton.Address{Status: proton.AddressStatusDisabled})
	if got.Status != int(proton.AddressStatusDisabled) {
		t.Fatalf("status mismatch: %d", got.Status)
	}
}
