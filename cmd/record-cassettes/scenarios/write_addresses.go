//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"os"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func init() {
	Register("create_delete_address", recordCreateDeleteAddress)
	Register("address_status_toggle", recordAddressStatusToggle)
	Register("update_address_display_name", recordUpdateAddressDisplayName)
}

// recordCreateDeleteAddress captures POST /core/v4/addresses/setup then
// DELETE /addresses/{id} for a throwaway alias on the custom domain.
// Precondition: RECORD_THROWAWAY_DOMAIN must be a VERIFIED custom domain on
// the account.
func recordCreateDeleteAddress(ctx context.Context) error {
	domain := os.Getenv("RECORD_THROWAWAY_DOMAIN")
	if domain == "" {
		return fmt.Errorf("RECORD_THROWAWAY_DOMAIN unset")
	}
	return recordRawTool(ctx, "create_delete_address", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
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
				return fmt.Errorf(
					"domain %q not found; add and verify it before recording",
					domain,
				)
			}
			addr, err := protonraw.CreateAddress(ctx, s.Raw(ctx), protonraw.CreateAddressRequest{
				DomainID:    domainID,
				LocalPart:   "record-test",
				DisplayName: "Record Test",
			})
			if err != nil {
				return fmt.Errorf("create address: %w", err)
			}
			c, err := s.Client(ctx)
			if err != nil {
				return fmt.Errorf("get client: %w", err)
			}
			if err := c.DeleteAddress(ctx, addr.ID); err != nil {
				return fmt.Errorf("delete address %s: %w", addr.ID, err)
			}
			return nil
		},
	)
}

// recordAddressStatusToggle creates an alias, disables it, enables it, then
// deletes it. Captures four operations: create, disable, enable, delete.
// Precondition: RECORD_THROWAWAY_DOMAIN must be a VERIFIED custom domain.
func recordAddressStatusToggle(ctx context.Context) error {
	domain := os.Getenv("RECORD_THROWAWAY_DOMAIN")
	if domain == "" {
		return fmt.Errorf("RECORD_THROWAWAY_DOMAIN unset")
	}
	return recordRawTool(ctx, "address_status_toggle", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
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
				return fmt.Errorf(
					"domain %q not found; add and verify it before recording",
					domain,
				)
			}
			addr, err := protonraw.CreateAddress(ctx, s.Raw(ctx), protonraw.CreateAddressRequest{
				DomainID:  domainID,
				LocalPart: "status-test",
			})
			if err != nil {
				return fmt.Errorf("create address: %w", err)
			}
			c, err := s.Client(ctx)
			if err != nil {
				return fmt.Errorf("get client: %w", err)
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
		},
	)
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
			if _, err := c.SetDisplayName(ctx, proton.SetDisplayNameReq{
				DisplayName: "Record Test Name",
			}); err != nil {
				return fmt.Errorf("set display name: %w", err)
			}
			_, err = c.SetDisplayName(ctx, proton.SetDisplayNameReq{
				DisplayName: original,
			})
			return err
		},
	)
}
