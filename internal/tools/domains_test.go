package tools

import (
	"testing"

	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
)

func TestToDomainDTO_FullPopulation(t *testing.T) {
	in := protonraw.CustomDomain{
		ID:          "d1",
		DomainName:  "example.com",
		State:       1,
		VerifyState: 2,
		MxState:     1,
		SpfState:    1,
		DkimState:   1,
		DmarcState:  0,
		Records: []protonraw.CustomDomainRecord{
			{Type: "TXT", Name: "@", Value: "proton-verification=abc", Purpose: "verify"},
			{Type: "MX", Name: "@", Value: "10 mail.protonmail.ch", Purpose: "mx"},
		},
	}
	got := toDomainDTO(in)
	if got.ID != "d1" || got.DomainName != "example.com" {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if got.VerifyState != 2 {
		t.Fatalf("verifyState mismatch: %d", got.VerifyState)
	}
	if len(got.Records) != 2 {
		t.Fatalf("records len: %d", len(got.Records))
	}
	rec := got.Records[0]
	if rec.Type != "TXT" || rec.Hostname != "@" || rec.Purpose != "verify" {
		t.Fatalf("record[0] mismatch: %+v", rec)
	}
}

func TestToDomainDTO_DefaultStates(t *testing.T) {
	tests := []struct {
		name        string
		in          protonraw.CustomDomain
		wantRecords int
	}{
		{"no_records", protonraw.CustomDomain{ID: "d2", DomainName: "b.example"}, 0},
		{"all_states_zero", protonraw.CustomDomain{ID: "d3"}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := toDomainDTO(tc.in)
			if len(got.Records) != tc.wantRecords {
				t.Fatalf("records len: got %d want %d", len(got.Records), tc.wantRecords)
			}
			if got.State != 0 || got.VerifyState != 0 || got.MxState != 0 ||
				got.SpfState != 0 || got.DkimState != 0 || got.DmarcState != 0 {
				t.Fatalf("zero-state mismatch: %+v", got)
			}
		})
	}
}
