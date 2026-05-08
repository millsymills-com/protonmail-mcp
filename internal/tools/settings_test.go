package tools

import (
	"testing"

	proton "github.com/ProtonMail/go-proton-api"
)

func TestToMailSettingsDTO_AllFields(t *testing.T) {
	got := toMailSettingsDTO(proton.MailSettings{
		DisplayName:     "Andy",
		Signature:       "<p>sig</p>",
		DraftMIMEType:   "text/html",
		AttachPublicKey: proton.Bool(true),
		Sign:            1,
		PGPScheme:       2,
	})
	if got.DisplayName != "Andy" || got.Signature != "<p>sig</p>" {
		t.Fatalf("scalar mismatch: %+v", got)
	}
	if got.DraftMIMEType != "text/html" {
		t.Fatalf("mime mismatch: %s", got.DraftMIMEType)
	}
	if !got.AttachPublicKey {
		t.Fatal("AttachPublicKey must be true")
	}
	if got.SignExternal != 1 || got.PGPScheme != 2 {
		t.Fatalf("enum mismatch: %+v", got)
	}
}

func TestToCoreSettingsDTO(t *testing.T) {
	got := toCoreSettingsDTO(proton.UserSettings{
		Telemetry:    proton.SettingEnabled,
		CrashReports: proton.SettingDisabled,
	})
	if got.Telemetry != int(proton.SettingEnabled) {
		t.Fatalf("telemetry mismatch: %d", got.Telemetry)
	}
	if got.CrashReports != int(proton.SettingDisabled) {
		t.Fatalf("crash mismatch: %d", got.CrashReports)
	}
}

func TestBoolToSettingsBool(t *testing.T) {
	tests := []struct {
		name string
		in   bool
		want proton.SettingsBool
	}{
		{"true_enabled", true, proton.SettingEnabled},
		{"false_disabled", false, proton.SettingDisabled},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := boolToSettingsBool(tc.in); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
}
