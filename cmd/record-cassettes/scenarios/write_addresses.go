//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/keychain"
	"github.com/millsymills-com/protonmail-mcp/internal/protonraw"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
	"github.com/millsymills-com/protonmail-mcp/internal/testvcr"
)

// writeAddressFixtureDomainID is the placeholder consumer tests pass as the
// `domain_id` parameter to proton_create_address. Address scenarios are
// recorded via injectors (account lacks `organization` scope to verify a real
// custom domain), so the recorder uses this same placeholder when calling
// protonraw.CreateAddress.
const writeAddressFixtureDomainID = "REDACTED_DOMAINID_1"

func registerWriteAddresses() {
	Register("create_delete_address", recordCreateDeleteAddress)
	Register("address_status_toggle", recordAddressStatusToggle)
	Register("update_address_display_name", recordUpdateAddressDisplayName)
}

// openWriteAddressCassette opens a recorder for write_addresses scenarios
// with the given chain of synthetic responses installed.
func openWriteAddressCassette(scenario string, inject func(http.RoundTripper) http.RoundTripper) (http.RoundTripper, func() error, error) {
	target := filepath.Join(toolsCassetteDir, scenario)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, nil, err
	}
	return testvcr.NewAtPath(target, testvcr.ModeRecord,
		testvcr.WithRealTransport(inject(http.DefaultTransport)))
}

// recordCreateDeleteAddress synthesizes POST /core/v4/addresses/setup +
// DELETE /addresses/{id}. Injectors stand in for the real API because the
// account lacks the `organization` scope needed to verify a real custom
// domain on which to create the alias.
func recordCreateDeleteAddress(ctx context.Context) (retErr error) {
	rt, stop, err := openWriteAddressCassette("create_delete_address",
		func(rt http.RoundTripper) http.RoundTripper {
			// Order matters: /core/v4/addresses/setup must be matched by the
			// outer wrapper. If the broader /core/v4/addresses/ (the
			// disable/enable/delete handler) wrapped /setup, it would steal
			// the create request because its substring is also a prefix.
			rt = inject200Ok(rt, "/core/v4/addresses/")
			rt = inject200AddressCreated(rt, "/core/v4/addresses/setup")
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
	c, clientErr := sess.Client(ctx)
	if clientErr != nil {
		return fmt.Errorf("get client: %w", clientErr)
	}
	addr, err := protonraw.CreateAddress(ctx, sess.Raw(ctx), protonraw.CreateAddressRequest{
		DomainID:    writeAddressFixtureDomainID,
		LocalPart:   "record-test",
		DisplayName: "Record Test",
	})
	if err != nil {
		return fmt.Errorf("create address: %w", err)
	}
	if err := c.DeleteAddress(ctx, addr.ID); err != nil {
		return fmt.Errorf("delete address %s: %w", addr.ID, err)
	}
	return nil
}

// recordAddressStatusToggle synthesizes create -> disable -> enable -> delete
// for an alias. Same rationale as recordCreateDeleteAddress on injector use.
func recordAddressStatusToggle(ctx context.Context) (retErr error) {
	rt, stop, err := openWriteAddressCassette("address_status_toggle",
		func(rt http.RoundTripper) http.RoundTripper {
			// Order matters: /core/v4/addresses/setup must be matched by the
			// outer wrapper. If the broader /core/v4/addresses/ (the
			// disable/enable/delete handler) wrapped /setup, it would steal
			// the create request because its substring is also a prefix.
			rt = inject200Ok(rt, "/core/v4/addresses/")
			rt = inject200AddressCreated(rt, "/core/v4/addresses/setup")
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
	c, clientErr := sess.Client(ctx)
	if clientErr != nil {
		return fmt.Errorf("get client: %w", clientErr)
	}
	addr, err := protonraw.CreateAddress(ctx, sess.Raw(ctx), protonraw.CreateAddressRequest{
		DomainID:  writeAddressFixtureDomainID,
		LocalPart: "status-test",
	})
	if err != nil {
		return fmt.Errorf("create address: %w", err)
	}
	if err := c.DisableAddress(ctx, addr.ID); err != nil {
		return fmt.Errorf("disable address: %w", err)
	}
	if err := c.EnableAddress(ctx, addr.ID); err != nil {
		return fmt.Errorf("enable address: %w", err)
	}
	if err := c.DeleteAddress(ctx, addr.ID); err != nil {
		return fmt.Errorf("delete address: %w", err)
	}
	return nil
}

// recordUpdateAddressDisplayName sets a new display name then restores the
// original. SetDisplayName is a global mail setting — the address ID parameter
// accepted by proton_update_address is cosmetic.
func recordUpdateAddressDisplayName(ctx context.Context) error {
	return recordReadTool(ctx, "update_address_display_name", toolsCassetteDir,
		func(c *proton.Client) error {
			ms, err := c.GetMailSettings(ctx)
			if err != nil {
				return fmt.Errorf("get mail settings: %w", err)
			}
			original := ms.DisplayName
			if _, setErr := c.SetDisplayName(ctx, proton.SetDisplayNameReq{
				DisplayName: "Record Test Name",
			}); setErr != nil {
				return fmt.Errorf("set display name: %w", setErr)
			}
			_, err = c.SetDisplayName(ctx, proton.SetDisplayNameReq{
				DisplayName: original,
			})
			return err
		},
	)
}
