//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	proton "github.com/ProtonMail/go-proton-api"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func init() {
	Register("error_captcha", recordErrorCaptcha)
	Register("error_rate_limited", recordErrorRateLimited)
	Register("error_not_found_message", recordErrorNotFoundMessage)
	Register("error_not_found_address", recordErrorNotFoundAddress)
	Register("error_not_found_domain", recordErrorNotFoundDomain)
	Register("error_permission_denied", recordErrorPermissionDenied)
	Register("error_conflict_add_domain", recordErrorConflictAddDomain)
	Register("error_validation_create_address", recordErrorValidationCreateAddress)
	Register("error_upstream_502", recordErrorUpstream502)
	Register("error_upstream_503", recordErrorUpstream503)
}

func openErrorCassette(scenario string) (http.RoundTripper, func() error, error) {
	target := filepath.Join(toolsCassetteDir, scenario)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, nil, err
	}
	return testvcr.NewAtPath(target, testvcr.ModeRecord)
}

// recordInjectedError is shared by injector-based error scenarios. It logs in,
// wraps the recorder transport with an injector, calls fn (which fires the
// injected error), then logs out. The cassette captures the synthetic response
// so replay does not require a live API.
func recordInjectedError(
	ctx context.Context,
	scenario string,
	inject func(rt http.RoundTripper) http.RoundTripper,
	fn func(c *proton.Client) error,
) (retErr error) {
	rt, stop, err := openErrorCassette(scenario)
	if err != nil {
		return err
	}
	defer func() {
		if err := stop(); err != nil && retErr == nil {
			retErr = err
		}
	}()

	kc := keychain.New()
	plainSess, err := loginAndPersistSession(ctx, kc)
	if err != nil {
		return err
	}
	defer logoutAndClear(plainSess, kc)

	wrapped := inject(rt)
	sess := session.New(defaultAPIURL(), kc, session.WithTransport(wrapped))
	c, err := sess.Client(ctx)
	if err != nil {
		return err
	}
	_ = fn(c) // error path is the point; ignore
	return nil
}

func recordErrorCaptcha(ctx context.Context) error {
	return recordInjectedError(ctx, "error_captcha",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject422Captcha(rt, "/core/v4/users")
		},
		func(c *proton.Client) error {
			_, err := c.GetUser(ctx)
			return err
		},
	)
}

func recordErrorRateLimited(ctx context.Context) error {
	return recordInjectedError(ctx, "error_rate_limited",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject429RateLimited(rt, "/core/v4/users")
		},
		func(c *proton.Client) error {
			_, err := c.GetUser(ctx)
			return err
		},
	)
}

// recordErrorNotFoundMessage captures the real 404 Proton returns for a
// nonexistent message ID. No injector — the live API generates the 404.
func recordErrorNotFoundMessage(ctx context.Context) error {
	return recordReadTool(ctx, "error_not_found_message", toolsCassetteDir,
		func(c *proton.Client) error {
			_, _ = c.GetMessage(ctx, "nonexistent") // ignore error — recording the 404 path
			return nil
		},
	)
}

// recordErrorNotFoundAddress captures the real 404 for a nonexistent address ID.
func recordErrorNotFoundAddress(ctx context.Context) error {
	return recordReadTool(ctx, "error_not_found_address", toolsCassetteDir,
		func(c *proton.Client) error {
			_, _ = c.GetAddress(ctx, "nonexistent") // ignore error — recording the 404 path
			return nil
		},
	)
}

// recordErrorNotFoundDomain captures the real 404 for a nonexistent domain ID.
func recordErrorNotFoundDomain(ctx context.Context) error {
	return recordRawTool(ctx, "error_not_found_domain", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			_, _ = protonraw.GetCustomDomain(ctx, s.Raw(ctx), "nonexistent") // ignore error
			return nil
		},
	)
}

func recordErrorPermissionDenied(ctx context.Context) error {
	return recordInjectedError(ctx, "error_permission_denied",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject403Forbidden(rt, "/core/v4/users")
		},
		func(c *proton.Client) error {
			_, err := c.GetUser(ctx)
			return err
		},
	)
}

// recordErrorConflictAddDomain adds the same domain twice; the second add returns
// 409 Conflict which Proton generates naturally (no injector needed).
func recordErrorConflictAddDomain(ctx context.Context) error {
	domain := os.Getenv("RECORD_THROWAWAY_DOMAIN")
	if domain == "" {
		return fmt.Errorf("RECORD_THROWAWAY_DOMAIN unset")
	}
	return recordRawTool(ctx, "error_conflict_add_domain", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			added, err := protonraw.AddCustomDomain(ctx, s.Raw(ctx), domain)
			if err != nil {
				return fmt.Errorf("first add: %w", err)
			}
			_, _ = protonraw.AddCustomDomain(ctx, s.Raw(ctx), domain) // captures 409
			_ = protonraw.RemoveCustomDomain(ctx, s.Raw(ctx), added.ID)
			return nil
		},
	)
}

// recordErrorValidationCreateAddress captures the 422 Proton returns for an
// invalid local part (e.g. "--bad"). No injector — the live API rejects it.
func recordErrorValidationCreateAddress(ctx context.Context) error {
	return recordRawTool(ctx, "error_validation_create_address", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			domain := os.Getenv("RECORD_THROWAWAY_DOMAIN")
			if domain == "" {
				return fmt.Errorf("RECORD_THROWAWAY_DOMAIN unset")
			}
			domains, err := protonraw.ListCustomDomains(ctx, s.Raw(ctx))
			if err != nil {
				return fmt.Errorf("list domains: %w", err)
			}
			var domainID string
			for _, d := range domains {
				if d.DomainName == domain {
					domainID = d.ID
					break
				}
			}
			if domainID == "" {
				return fmt.Errorf("domain %q not found; add and verify it first", domain)
			}
			// ignore error — recording the 422 validation-error path
			_, _ = protonraw.CreateAddress(ctx, s.Raw(ctx), protonraw.CreateAddressRequest{
				DomainID:  domainID,
				LocalPart: "--bad",
			})
			return nil
		},
	)
}

func recordErrorUpstream502(ctx context.Context) error {
	return recordInjectedError(ctx, "error_upstream_502",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject502BadGateway(rt, "/core/v4/users")
		},
		func(c *proton.Client) error {
			_, err := c.GetUser(ctx)
			return err
		},
	)
}

func recordErrorUpstream503(ctx context.Context) error {
	return recordInjectedError(ctx, "error_upstream_503",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject503Unavailable(rt, "/core/v4/users")
		},
		func(c *proton.Client) error {
			_, err := c.GetUser(ctx)
			return err
		},
	)
}
