package tools

import (
	"reflect"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
)

func TestToAddressDTO(t *testing.T) {
	tests := []struct {
		name string
		in   proton.Address
		want addressDTO
	}{
		{
			name: "full-population",
			in: proton.Address{
				ID:          "addr-1",
				Email:       "a@example.com",
				DisplayName: "Andy",
				Status:      proton.AddressStatusEnabled,
				Order:       1,
				Type:        proton.AddressTypeOriginal,
				Send:        proton.Bool(true),
				Receive:     proton.Bool(true),
				Keys:        []proton.Key{{ID: "k1"}, {ID: "k2"}},
			},
			want: addressDTO{
				ID:          "addr-1",
				Email:       "a@example.com",
				DisplayName: "Andy",
				Status:      int(proton.AddressStatusEnabled),
				Order:       1,
				Type:        int(proton.AddressTypeOriginal),
				Send:        true,
				Receive:     true,
				KeyIDs:      []string{"k1", "k2"},
			},
		},
		{
			name: "empty-keys-yields-empty-slice",
			in:   proton.Address{ID: "x"},
			want: addressDTO{ID: "x", KeyIDs: []string{}},
		},
		{
			name: "disabled-status",
			in:   proton.Address{Status: proton.AddressStatusDisabled},
			want: addressDTO{Status: int(proton.AddressStatusDisabled), KeyIDs: []string{}},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toAddressDTO(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("toAddressDTO mismatch\n got: %+v\nwant: %+v", got, tc.want)
			}
			if tc.name == "empty-keys-yields-empty-slice" && got.KeyIDs == nil {
				t.Fatal("KeyIDs must be non-nil empty slice, not nil (matches make())")
			}
		})
	}
}
