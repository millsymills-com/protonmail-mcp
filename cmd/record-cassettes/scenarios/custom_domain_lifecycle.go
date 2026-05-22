//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/millsmillsymills/protonmail-mcp/internal/keychain"
	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
	"github.com/millsmillsymills/protonmail-mcp/internal/testvcr"
)

func registerCustomDomainLifecycle() {
	Register("add_remove_custom_domain", recordAddRemoveCustomDomain)
	Register("verify_custom_domain_pending", recordVerifyCustomDomainPending)
	Register("list_custom_domains_happy", recordListCustomDomainsHappy)
	Register("get_custom_domain_happy", recordGetCustomDomainHappy)
	Register("get_catchall_enabled", recordGetCatchallEnabled)
	Register("get_catchall_disabled", recordGetCatchallDisabled)
	Register("set_catchall_happy", recordSetCatchallHappy)
	Register("disable_catchall_happy", recordDisableCatchallHappy)
}

// openDomainCassette opens a recorder for custom-domain scenarios with the
// given injector chain installed. Custom-domain endpoints require the
// `organization` API scope (Proton Business plan); the user's account is
// Plus-tier, so all real /core/v4/domains calls return 403. Synthetic
// responses keep the consumer tests exercised without requiring a plan
// upgrade or a second test account.
func openDomainCassette(scenario string, inject func(http.RoundTripper) http.RoundTripper) (http.RoundTripper, func() error, error) {
	target := filepath.Join(toolsCassetteDir, scenario)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, nil, err
	}
	return testvcr.NewAtPath(target, testvcr.ModeRecord,
		testvcr.WithRealTransport(inject(http.DefaultTransport)))
}

// recordAddRemoveCustomDomain synthesizes POST /core/v4/domains then
// DELETE /core/v4/domains/{id}. See [openDomainCassette] for the rationale.
func recordAddRemoveCustomDomain(ctx context.Context) (retErr error) {
	rt, stop, err := openDomainCassette("add_remove_custom_domain",
		func(rt http.RoundTripper) http.RoundTripper {
			// Order matters: /core/v4/domains/<id> for DELETE needs the broad
			// wrapper to be on the outside; the inner exact-suffix matcher for
			// POST /core/v4/domains then catches the create call. The
			// create-injector returns a domain object; the broad one matches
			// any path containing /core/v4/domains/ which is the DELETE form.
			rt = inject200Ok(rt, "/core/v4/domains/")
			rt = inject200CustomDomainSingle(rt, "/core/v4/domains")
			return rt
		})
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
	added, err := protonraw.AddCustomDomain(ctx, sess.Raw(ctx), "example.test")
	if err != nil {
		return fmt.Errorf("add domain: %w", err)
	}
	if err := protonraw.RemoveCustomDomain(ctx, sess.Raw(ctx), added.ID); err != nil {
		return fmt.Errorf("remove domain %s: %w", added.ID, err)
	}
	return nil
}

// recordVerifyCustomDomainPending synthesizes GET /core/v4/domains (list)
// then PUT /core/v4/domains/{id}/verify.
func recordVerifyCustomDomainPending(ctx context.Context) (retErr error) {
	rt, stop, err := openDomainCassette("verify_custom_domain_pending",
		func(rt http.RoundTripper) http.RoundTripper {
			// /core/v4/domains/<id>/verify matches both the broad and single
			// patterns; arrange so the single (more specific by being deeper
			// in the chain) wins on PUT /<id>/verify.
			rt = inject200CustomDomainsList(rt, "/core/v4/domains")
			rt = inject200CustomDomainSingle(rt, "/verify")
			return rt
		})
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
	domains, err := protonraw.ListCustomDomains(ctx, sess.Raw(ctx))
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}
	if len(domains) == 0 {
		return fmt.Errorf("synthetic list returned no domains")
	}
	_, err = protonraw.VerifyCustomDomain(ctx, sess.Raw(ctx), domains[0].ID)
	return err
}

// recordListCustomDomainsHappy synthesizes GET /core/v4/domains.
func recordListCustomDomainsHappy(ctx context.Context) (retErr error) {
	rt, stop, err := openDomainCassette("list_custom_domains_happy",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject200CustomDomainsList(rt, "/core/v4/domains")
		})
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
	_, err = protonraw.ListCustomDomains(ctx, sess.Raw(ctx))
	return err
}

// recordGetCatchallEnabled synthesizes GET /core/v4/domains/{id}/addresses
// with one address whose CatchAll=true. The consumer tool reports the
// enabled catchall and its destination address.
func recordGetCatchallEnabled(ctx context.Context) (retErr error) {
	return recordCatchallScenario(ctx, "get_catchall_enabled",
		inject200DomainAddressesCatchallOn, func(s *session.Session) error {
			_, listErr := protonraw.ListDomainAddresses(ctx, s.Raw(ctx), "REDACTED_DOMAINID_1")
			return listErr
		})
}

// recordGetCatchallDisabled is the same shape but every address has
// CatchAll=false; the tool reports enabled=false.
func recordGetCatchallDisabled(ctx context.Context) (retErr error) {
	return recordCatchallScenario(ctx, "get_catchall_disabled",
		inject200DomainAddressesCatchallOff, func(s *session.Session) error {
			_, listErr := protonraw.ListDomainAddresses(ctx, s.Raw(ctx), "REDACTED_DOMAINID_1")
			return listErr
		})
}

// recordSetCatchallHappy synthesizes the GET addresses + PUT catchall pair
// proton_set_catchall makes when the destination address is on the domain.
func recordSetCatchallHappy(ctx context.Context) (retErr error) {
	rt, stop, err := openDomainCassette("set_catchall_happy",
		func(rt http.RoundTripper) http.RoundTripper {
			// /catchall is the PUT target; install its persistent 200 first
			// so it sits inside the broader /addresses listing wrapper.
			rt = inject200Ok(rt, "/catchall")
			rt = inject200DomainAddressesCatchallOff(rt, "/addresses")
			return rt
		})
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
	if _, listErr := protonraw.ListDomainAddresses(ctx, sess.Raw(ctx), "REDACTED_DOMAINID_1"); listErr != nil {
		return fmt.Errorf("list addresses: %w", listErr)
	}
	addrID := "REDACTED_ADDRESSID_1"
	if err := protonraw.UpdateCatchAll(ctx, sess.Raw(ctx), "REDACTED_DOMAINID_1", &addrID); err != nil {
		return fmt.Errorf("set catchall: %w", err)
	}
	return nil
}

// recordDisableCatchallHappy synthesizes the PUT catchall (null) call.
func recordDisableCatchallHappy(ctx context.Context) (retErr error) {
	rt, stop, err := openDomainCassette("disable_catchall_happy",
		func(rt http.RoundTripper) http.RoundTripper {
			return inject200Ok(rt, "/catchall")
		})
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
	if err := protonraw.UpdateCatchAll(ctx, sess.Raw(ctx), "REDACTED_DOMAINID_1", nil); err != nil {
		return fmt.Errorf("disable catchall: %w", err)
	}
	return nil
}

// recordCatchallScenario shares the cassette open + login + run shell for the
// two GET-catchall scenarios.
func recordCatchallScenario(
	ctx context.Context, scenario string,
	inject func(http.RoundTripper, string) http.RoundTripper,
	fn func(*session.Session) error,
) (retErr error) {
	rt, stop, err := openDomainCassette(scenario,
		func(rt http.RoundTripper) http.RoundTripper {
			return inject(rt, "/addresses")
		})
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
	return fn(sess)
}

// recordGetCustomDomainHappy synthesizes GET /core/v4/domains (list) then
// GET /core/v4/domains/{id} (single).
func recordGetCustomDomainHappy(ctx context.Context) (retErr error) {
	rt, stop, err := openDomainCassette("get_custom_domain_happy",
		func(rt http.RoundTripper) http.RoundTripper {
			rt = inject200CustomDomainSingle(rt, "/core/v4/domains/")
			rt = inject200CustomDomainsList(rt, "/core/v4/domains")
			return rt
		})
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
	domains, err := protonraw.ListCustomDomains(ctx, sess.Raw(ctx))
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}
	if len(domains) == 0 {
		return fmt.Errorf("synthetic list returned no domains")
	}
	_, err = protonraw.GetCustomDomain(ctx, sess.Raw(ctx), domains[0].ID)
	return err
}
