package tools

import (
	"context"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/proterr"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// mailSettingsDTO mirrors the actual proton.MailSettings shape. Note: the
// upstream type does NOT expose AutoSaveContacts / HideEmbeddedImages /
// HideRemoteImages — those fields existed in the planning spec but have been
// removed from go-proton-api.
type mailSettingsDTO struct {
	DisplayName     string `json:"display_name"`
	Signature       string `json:"signature"`
	DraftMIMEType   string `json:"draft_mime_type"`
	AttachPublicKey bool   `json:"attach_public_key"`
	SignExternal    int    `json:"sign_external_messages"`
	PGPScheme       int    `json:"default_pgp_scheme"`
}

// coreSettingsDTO mirrors the actual proton.UserSettings shape, which only
// exposes Telemetry and CrashReports today (Locale, WeeklyEmail, News are not
// present). Values are SettingsBool — 0=disabled, 1=enabled.
type coreSettingsDTO struct {
	Telemetry    int `json:"telemetry"`
	CrashReports int `json:"crash_reports"`
}

type getMailSettingsIn struct{}
type getMailSettingsOut struct {
	Settings mailSettingsDTO `json:"settings"`
}

type getCoreSettingsIn struct{}
type getCoreSettingsOut struct {
	Settings coreSettingsDTO `json:"settings"`
}

type updateMailSettingsIn struct {
	DisplayName *string `json:"display_name,omitempty"`
	Signature   *string `json:"signature,omitempty"`
}
type updateMailSettingsOut struct {
	Settings mailSettingsDTO `json:"settings"`
}

type updateCoreSettingsIn struct {
	Telemetry    *bool `json:"telemetry,omitempty" jsonschema:"enable or disable Proton telemetry"`
	CrashReports *bool `json:"crash_reports,omitempty" jsonschema:"enable or disable crash reports"`
}
type updateCoreSettingsOut struct {
	Settings coreSettingsDTO `json:"settings"`
}

func registerSettings(server *mcp.Server, d Deps) {
	addTool(server, d, &mcp.Tool{
		Name:        "proton_get_mail_settings",
		Description: "Returns mail settings (display name, signature, draft MIME type, default PGP scheme).",
	}, func(ctx context.Context, d Deps, _ getMailSettingsIn) (getMailSettingsOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return getMailSettingsOut{}, perr
		}
		ms, err := c.GetMailSettings(ctx)
		if err != nil {
			return getMailSettingsOut{}, proterr.Map(err)
		}
		return getMailSettingsOut{Settings: toMailSettingsDTO(ms)}, nil
	})

	addTool(server, d, &mcp.Tool{
		Name:        "proton_get_core_settings",
		Description: "Returns account-level (core) settings: telemetry and crash-report flags.",
	}, func(ctx context.Context, d Deps, _ getCoreSettingsIn) (getCoreSettingsOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return getCoreSettingsOut{}, perr
		}
		us, err := c.GetUserSettings(ctx)
		if err != nil {
			return getCoreSettingsOut{}, proterr.Map(err)
		}
		return getCoreSettingsOut{Settings: toCoreSettingsDTO(us)}, nil
	})

	if !WritesEnabled() {
		return
	}

	addTool(server, d, &mcp.Tool{
		Name:        "proton_update_mail_settings",
		Description: "Updates mail settings (partial — only set fields are changed).",
	}, func(ctx context.Context, d Deps, in updateMailSettingsIn) (updateMailSettingsOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return updateMailSettingsOut{}, perr
		}
		var ms proton.MailSettings
		var err error
		if in.DisplayName != nil {
			ms, err = c.SetDisplayName(ctx, proton.SetDisplayNameReq{DisplayName: *in.DisplayName})
			if err != nil {
				return updateMailSettingsOut{}, proterr.Map(err)
			}
		}
		if in.Signature != nil {
			ms, err = c.SetSignature(ctx, proton.SetSignatureReq{Signature: *in.Signature})
			if err != nil {
				return updateMailSettingsOut{}, proterr.Map(err)
			}
		}
		if in.DisplayName == nil && in.Signature == nil {
			ms, err = c.GetMailSettings(ctx)
			if err != nil {
				return updateMailSettingsOut{}, proterr.Map(err)
			}
		}
		return updateMailSettingsOut{Settings: toMailSettingsDTO(ms)}, nil
	})

	addTool(server, d, &mcp.Tool{
		Name:        "proton_update_core_settings",
		Description: "Updates account-level (core) settings: telemetry and crash-report toggles.",
	}, func(ctx context.Context, d Deps, in updateCoreSettingsIn) (updateCoreSettingsOut, *proterr.Error) {
		c, perr := client(ctx, d)
		if perr != nil {
			return updateCoreSettingsOut{}, perr
		}
		var us proton.UserSettings
		var err error
		if in.Telemetry != nil {
			us, err = c.SetUserSettingsTelemetry(ctx, proton.SetTelemetryReq{Telemetry: boolToSettingsBool(*in.Telemetry)})
			if err != nil {
				return updateCoreSettingsOut{}, proterr.Map(err)
			}
		}
		if in.CrashReports != nil {
			us, err = c.SetUserSettingsCrashReports(ctx, proton.SetCrashReportReq{CrashReports: boolToSettingsBool(*in.CrashReports)})
			if err != nil {
				return updateCoreSettingsOut{}, proterr.Map(err)
			}
		}
		if in.Telemetry == nil && in.CrashReports == nil {
			us, err = c.GetUserSettings(ctx)
			if err != nil {
				return updateCoreSettingsOut{}, proterr.Map(err)
			}
		}
		return updateCoreSettingsOut{Settings: toCoreSettingsDTO(us)}, nil
	})
}

func toMailSettingsDTO(m proton.MailSettings) mailSettingsDTO {
	return mailSettingsDTO{
		DisplayName:     m.DisplayName,
		Signature:       m.Signature,
		DraftMIMEType:   string(m.DraftMIMEType),
		AttachPublicKey: bool(m.AttachPublicKey),
		SignExternal:    int(m.Sign),
		PGPScheme:       int(m.PGPScheme),
	}
}

func toCoreSettingsDTO(u proton.UserSettings) coreSettingsDTO {
	return coreSettingsDTO{
		Telemetry:    int(u.Telemetry),
		CrashReports: int(u.CrashReports),
	}
}

func boolToSettingsBool(b bool) proton.SettingsBool {
	if b {
		return proton.SettingEnabled
	}
	return proton.SettingDisabled
}
