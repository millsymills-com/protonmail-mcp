package tools

import (
	"net/mail"
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
)

func TestFormatAddressWithName(t *testing.T) {
	got := formatAddress(&mail.Address{Name: "Andy", Address: "andy@example.com"})
	if got != "Andy <andy@example.com>" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAddressBareEmail(t *testing.T) {
	got := formatAddress(&mail.Address{Address: "andy@example.com"})
	if got != "andy@example.com" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAddressNil(t *testing.T) {
	if got := formatAddress(nil); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatAddressesSkipsEmpty(t *testing.T) {
	got := formatAddresses([]*mail.Address{
		{Address: "a@example.com"},
		nil,
		{Name: "B", Address: "b@example.com"},
	})
	if len(got) != 2 || got[0] != "a@example.com" || got[1] != "B <b@example.com>" {
		t.Fatalf("got %v", got)
	}
}

func TestFilterSensitiveHeadersDropsBccAndIPHeaders(t *testing.T) {
	in := map[string][]string{
		"Authentication-Results": {"spf=pass; dkim=pass"},
		"DKIM-Signature":         {"v=1; a=rsa-sha256; ..."},
		"Bcc":                    {"hidden@example.com"},
		"BCC":                    {"another@example.com"},
		"X-Originating-IP":       {"[203.0.113.5]"},
		"X-Original-Sender":      {"sender@example.com"},
		"X-Real-Ip":              {"203.0.113.5"},
		"Received":               {"from mail.example.com"},
	}
	got := filterSensitiveHeaders(in)
	for _, k := range []string{"Bcc", "BCC", "X-Originating-IP", "X-Original-Sender", "X-Real-Ip"} {
		if _, ok := got[k]; ok {
			t.Errorf("header %q must be filtered out, got %v", k, got[k])
		}
	}
	for _, k := range []string{"Authentication-Results", "DKIM-Signature", "Received"} {
		if _, ok := got[k]; !ok {
			t.Errorf("header %q must pass through, got: %v", k, got)
		}
	}
}

func TestFilterSensitiveHeadersNilOnEmpty(t *testing.T) {
	if got := filterSensitiveHeaders(nil); got != nil {
		t.Fatalf("nil input must return nil, got: %v", got)
	}
	if got := filterSensitiveHeaders(map[string][]string{}); got != nil {
		t.Fatalf("empty input must return nil, got: %v", got)
	}
}

func TestToMessageStubDTOPopulatesFields(t *testing.T) {
	meta := proton.MessageMetadata{
		ID:             "m1",
		AddressID:      "a1",
		LabelIDs:       []string{"L1", "L2"},
		Subject:        "test",
		Sender:         &mail.Address{Address: "from@example.com"},
		ToList:         []*mail.Address{{Address: "to@example.com"}},
		Time:           1714000000,
		Unread:         true,
		NumAttachments: 2,
	}
	dto := toMessageStubDTO(meta)
	if dto.ID != "m1" || dto.From != "from@example.com" || dto.InternalDate != 1714000000 {
		t.Fatalf("scalar fields wrong: %+v", dto)
	}
	if len(dto.To) != 1 || dto.To[0] != "to@example.com" {
		t.Fatalf("To wrong: %v", dto.To)
	}
	if !dto.Unread || !dto.HasAttachment {
		t.Fatalf("flags wrong: unread=%v has_attach=%v", dto.Unread, dto.HasAttachment)
	}
	if len(dto.LabelIDs) != 2 {
		t.Fatalf("LabelIDs wrong: %v", dto.LabelIDs)
	}
}
