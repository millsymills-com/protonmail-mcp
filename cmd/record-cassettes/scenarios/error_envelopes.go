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
// to any real message, address, or domain. Proton validates ID shape *and*
// content before the lookup; an all-zeros literal (e.g. "AAAA...==") trips
// the format check and yields 400 Code=2061 "Attribute ID is invalid"
// instead of the 404 the consumer tests assert on. The mixed bytes below
// pass shape validation and reliably miss every real resource.
const nonexistentResourceID = "Zk9uX1ZjUjBudGUtTm90QVJlYWxJRC1OZWl0aGVySXNUaGlzT25lLU5vclRoYXRPbmUtSnVzdEZpbGxlcg=="

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
//
// Consumer tests build their proton.Client via sess.Client(ctx) which does a
// cold-start refresh against /auth/v4/refresh on every test boot. The cassette
// must therefore include a refresh interaction before the synthetic-error
// interaction. Using sess.Client here (vs. the seed pattern used by
// refresh_revoked) records exactly that pair.
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
	if _, loginErr := freshLoginForScenario(ctx, kc); loginErr != nil {
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

// recordErrorNotFoundMessage captures a 404 for a nonexistent message ID.
// Proton's real API returns 400 "Attribute ID is invalid" for any opaque ID
// that doesn't pass its internal shape/content validation, even for clearly
// nonexistent values — there's no ID format we can construct that yields a
// genuine 404. Inject the 404 instead, like the captcha/rate_limit/upstream
// scenarios already do.
func recordErrorNotFoundMessage(ctx context.Context) error {
	return recordInjectedError(ctx, "error_not_found_message",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject404NotFound(rt, "/mail/v4/messages/")
		},
		func(c *proton.Client) error {
			_, err := c.GetMessage(ctx, nonexistentResourceID)
			return err
		},
	)
}

// recordErrorNotFoundAddress: same pattern as recordErrorNotFoundMessage.
func recordErrorNotFoundAddress(ctx context.Context) error {
	return recordInjectedError(ctx, "error_not_found_address",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject404NotFound(rt, "/core/v4/addresses/")
		},
		func(c *proton.Client) error {
			_, err := c.GetAddress(ctx, nonexistentResourceID)
			return err
		},
	)
}

// recordErrorNotFoundDomain: same pattern. Goes through the raw client because
// custom-domain APIs aren't on proton.Client.
func recordErrorNotFoundDomain(ctx context.Context) (retErr error) {
	scenario := "error_not_found_domain"
	rt, stop, err := openErrorCassette(scenario,
		testvcr.WithRealTransport(inject404NotFound(http.DefaultTransport, "/core/v4/domains/")))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	kc := keychain.New()
	if _, loginErr := freshLoginForScenario(ctx, kc); loginErr != nil {
		return loginErr
	}
	sess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	if _, clientErr := sess.Client(ctx); clientErr != nil {
		return fmt.Errorf("get client: %w", clientErr)
	}
	_, _ = protonraw.GetCustomDomain(ctx, sess.Raw(ctx), nonexistentResourceID)
	return nil
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

// recordErrorConflictAddDomain synthesizes the 200-then-409 sequence Proton
// returns when a domain is added twice. Injectors are used instead of the
// real API because the account doesn't have the `organization` scope needed
// for /core/v4/domains POST (only Proton Business plans grant it).
//
// The recorder stacks two one-shot injectors: the outer (200 added) fires on
// the first POST and is consumed; the second POST passes through to the inner
// (409 conflict).
func recordErrorConflictAddDomain(ctx context.Context) (retErr error) {
	scenario := "error_conflict_add_domain"
	injected := inject409ConflictDomainAlreadyExists(http.DefaultTransport, "/core/v4/domains")
	injected = inject200DomainAdded(injected, "/core/v4/domains")
	rt, stop, err := openErrorCassette(scenario, testvcr.WithRealTransport(injected))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	kc := keychain.New()
	if _, loginErr := freshLoginForScenario(ctx, kc); loginErr != nil {
		return loginErr
	}
	sess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	if _, clientErr := sess.Client(ctx); clientErr != nil {
		return fmt.Errorf("get client: %w", clientErr)
	}
	if _, addErr := protonraw.AddCustomDomain(ctx, sess.Raw(ctx), "example.test"); addErr != nil {
		return fmt.Errorf("first add: %w", addErr)
	}
	_, _ = protonraw.AddCustomDomain(ctx, sess.Raw(ctx), "example.test") // captures 409
	return nil
}

// recordErrorValidationCreateAddress synthesizes the 422 Proton returns for
// an invalid local part. Real API can't be used because creating an address
// needs a verified custom domain on the account, which in turn needs the
// `organization` scope this account lacks.
func recordErrorValidationCreateAddress(ctx context.Context) (retErr error) {
	scenario := "error_validation_create_address"
	injected := inject422ValidationInvalidLocalPart(http.DefaultTransport, "/core/v4/addresses/setup")
	rt, stop, err := openErrorCassette(scenario, testvcr.WithRealTransport(injected))
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := stop(); closeErr != nil && retErr == nil {
			retErr = closeErr
		}
	}()

	kc := keychain.New()
	if _, loginErr := freshLoginForScenario(ctx, kc); loginErr != nil {
		return loginErr
	}
	sess := session.New(defaultAPIURL(), kc, session.WithTransport(rt))
	if _, clientErr := sess.Client(ctx); clientErr != nil {
		return fmt.Errorf("get client: %w", clientErr)
	}
	_, _ = protonraw.CreateAddress(ctx, sess.Raw(ctx), protonraw.CreateAddressRequest{
		DomainID:  "REDACTED_DOMAINID_1",
		LocalPart: "--bad",
	})
	return nil
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
