package tools

import "testing"

func TestValidateMessageIDsEmpty(t *testing.T) {
	if perr := validateMessageIDs(nil); perr == nil || perr.Code != "proton/validation" {
		t.Fatalf("want validation error for empty ids, got %+v", perr)
	}
}

func TestValidateMessageIDsPresent(t *testing.T) {
	if perr := validateMessageIDs([]string{"id1"}); perr != nil {
		t.Fatalf("want nil for non-empty ids, got %+v", perr)
	}
}

func TestValidateLabelAction(t *testing.T) {
	for _, a := range []string{"add", "remove"} {
		if perr := validateLabelAction(a); perr != nil {
			t.Fatalf("want nil for %q, got %+v", a, perr)
		}
	}
	if perr := validateLabelAction("toggle"); perr == nil || perr.Code != "proton/validation" {
		t.Fatalf("want validation error for bad action, got %+v", perr)
	}
}
