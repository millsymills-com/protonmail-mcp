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

// nonexistentResourceID is a syntactically-valid Proton opaque resource ID
// (88-char URL-safe base64 with two padding chars) that does not correspond
// to any real message, address, or domain. Proton validates ID shape before
// the lookup; passing a short literal like nonexistentResourceID trips the format
// check and yields 400 Code=2061 "Attribute ID is invalid" rather than the
// 404 the consumer tests assert on.
const nonexistentResourceID = "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=="

func registerErrorEnvelopes() {
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

func openErrorCassette(scenario string, opts ...testvcr.Option) (http.RoundTripper, func() error, error) {
	target := filepath.Join(toolsCassetteDir, scenario)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, nil, err
	}
	return testvcr.NewAtPath(target, testvcr.ModeRecord, opts...)
}

// recordInjectedError is shared by injector-based error scenarios. The
// injector is installed as the recorder's upstream transport so its
// synthetic response is captured into the cassette through go-vcr's normal
// record path; pre-#87 it wrapped the recorder externally and short-circuited
// the cassette write.
func recordInjectedError(
	ctx context.Context,
	scenario string,
	inject func(rt http.RoundTripper) http.RoundTripper,
	fn func(c *proton.Client) error,
) (retErr error) {
	rt, stop, err := openErrorCassette(scenario, testvcr.WithRealTransport(inject(http.DefaultTransport)))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	kc := keychain.New()
	if _, loginErr := loginAndPersistSession(ctx, kc); loginErr != nil {
		return loginErr
	}

	sess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
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
			_, _ = c.GetMessage(ctx, nonexistentResourceID) // ignore error — recording the 404 path
			return nil
		},
	)
}

// recordErrorNotFoundAddress captures the real 404 for a nonexistent address ID.
func recordErrorNotFoundAddress(ctx context.Context) error {
	return recordReadTool(ctx, "error_not_found_address", toolsCassetteDir,
		func(c *proton.Client) error {
			_, _ = c.GetAddress(ctx, nonexistentResourceID) // ignore error — recording the 404 path
			return nil
		},
	)
}

// recordErrorNotFoundDomain captures the real 404 for a nonexistent domain ID.
func recordErrorNotFoundDomain(ctx context.Context) error {
	return recordRawTool(ctx, "error_not_found_domain", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			_, _ = protonraw.GetCustomDomain(ctx, s.Raw(ctx), nonexistentResourceID) // ignore error
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
