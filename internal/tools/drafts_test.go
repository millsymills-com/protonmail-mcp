package tools

import (
	"strings"
	"testing"

	"github.com/ProtonMail/gluon/rfc822"
)

func TestParseRecipientsValid(t *testing.T) {
	got, perr := parseRecipients([]string{"a@example.test", "Bob <b@example.test>"})
	if perr != nil {
		t.Fatalf("unexpected error: %+v", perr)
	}
	if len(got) != 2 || got[0].Address != "a@example.test" || got[1].Name != "Bob" {
		t.Fatalf("bad parse: %#v", got)
	}
}

func TestParseRecipientsEmptyIsNil(t *testing.T) {
	got, perr := parseRecipients(nil)
	if perr != nil || got != nil {
		t.Fatalf("want (nil,nil), got (%#v,%+v)", got, perr)
	}
}

func TestParseRecipientsMalformedErrors(t *testing.T) {
	_, perr := parseRecipients([]string{"not-an-email"})
	if perr == nil || perr.Code != "proton/validation" {
		t.Fatalf("want proton/validation, got %+v", perr)
	}
}

func TestResolveMIMETypeDefaultAndOverride(t *testing.T) {
	def, perr := resolveMIMEType("")
	if perr != nil || def != rfc822.TextPlain {
		t.Fatalf("want TextPlain default, got %v %+v", def, perr)
	}
	html, perr := resolveMIMEType("text/html")
	if perr != nil || html != rfc822.TextHTML {
		t.Fatalf("want TextHTML, got %v %+v", html, perr)
	}
	if _, perr := resolveMIMEType("application/json"); perr == nil || perr.Code != "proton/validation" {
		t.Fatalf("want validation error for unsupported mime, got %+v", perr)
	}
}

func TestParseRecipientsErrorNamesBadEntry(t *testing.T) {
	_, perr := parseRecipients([]string{"a@example.test", "bogus"})
	if perr == nil || !strings.Contains(perr.Message, `"bogus"`) {
		t.Fatalf("want error naming the bad entry, got %+v", perr)
	}
}

func TestResolveMIMETypeExplicitPlain(t *testing.T) {
	mt, perr := resolveMIMEType("text/plain")
	if perr != nil || mt != rfc822.TextPlain {
		t.Fatalf("want TextPlain, got %v %+v", mt, perr)
	}
}

func TestQuoteTruncCapsOversizedInput(t *testing.T) {
	in := strings.Repeat("a", 150)
	got := quoteTrunc(in)
	if !strings.Contains(got, "…") {
		t.Fatalf("want truncation marker in %q", got)
	}
	if !strings.HasPrefix(got, `"`) || !strings.HasSuffix(got, `"`) {
		t.Fatalf("want quoted output, got %q", got)
	}
	if len(got) > 120 {
		t.Fatalf("output not capped: %d chars", len(got))
	}
}
