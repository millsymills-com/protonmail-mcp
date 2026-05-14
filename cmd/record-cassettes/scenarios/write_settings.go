//go:build recording

package scenarios

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
)

func init() {
	Register("update_mail_settings_signature", recordUpdateMailSettingsSignature)
	Register("update_core_settings_flags", recordUpdateCoreSettingsFlags)
}

// recordUpdateMailSettingsSignature sets a new signature then restores the
// original. Captures GetMailSettings, SetSignature (new), SetSignature (restore).
func recordUpdateMailSettingsSignature(ctx context.Context) error {
	return recordReadTool(ctx, "update_mail_settings_signature", toolsCassetteDir,
		func(c *proton.Client) error {
			ms, err := c.GetMailSettings(ctx)
			if err != nil {
				return fmt.Errorf("get mail settings: %w", err)
			}
			original := ms.Signature
			if _, err := c.SetSignature(ctx, proton.SetSignatureReq{
				Signature: "<p>Record test signature</p>",
			}); err != nil {
				return fmt.Errorf("set signature: %w", err)
			}
			_, err = c.SetSignature(ctx, proton.SetSignatureReq{Signature: original})
			return err
		},
	)
}

// recordUpdateCoreSettingsFlags toggles telemetry off then on, and crash
// reports off then on. Captures two SetTelemetry and two SetCrashReport calls.
func recordUpdateCoreSettingsFlags(ctx context.Context) error {
	return recordReadTool(ctx, "update_core_settings_flags", toolsCassetteDir,
		func(c *proton.Client) error {
			us, err := c.GetUserSettings(ctx)
			if err != nil {
				return fmt.Errorf("get user settings: %w", err)
			}
			origTelemetry := us.Telemetry
			origCrash := us.CrashReports

			off := proton.SettingDisabled
			on := proton.SettingEnabled

			if _, err := c.SetUserSettingsTelemetry(ctx,
				proton.SetTelemetryReq{Telemetry: off}); err != nil {
				return fmt.Errorf("disable telemetry: %w", err)
			}
			if _, err := c.SetUserSettingsCrashReports(ctx,
				proton.SetCrashReportReq{CrashReports: off}); err != nil {
				return fmt.Errorf("disable crash reports: %w", err)
			}
			if _, err := c.SetUserSettingsTelemetry(ctx,
				proton.SetTelemetryReq{Telemetry: on}); err != nil {
				return fmt.Errorf("re-enable telemetry: %w", err)
			}
			if _, err := c.SetUserSettingsCrashReports(ctx,
				proton.SetCrashReportReq{CrashReports: on}); err != nil {
				return fmt.Errorf("re-enable crash reports: %w", err)
			}

			// Restore actual originals (in case they were already off).
			if origTelemetry == off {
				if _, err := c.SetUserSettingsTelemetry(ctx,
					proton.SetTelemetryReq{Telemetry: off}); err != nil {
					return fmt.Errorf("restore telemetry: %w", err)
				}
			}
			if origCrash == off {
				if _, err := c.SetUserSettingsCrashReports(ctx,
					proton.SetCrashReportReq{CrashReports: off}); err != nil {
					return fmt.Errorf("restore crash reports: %w", err)
				}
			}
			return nil
		},
	)
}
