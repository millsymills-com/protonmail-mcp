//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"os"

	"github.com/millsmillsymills/protonmail-mcp/internal/protonraw"
	"github.com/millsmillsymills/protonmail-mcp/internal/session"
)

func init() {
	Register("add_remove_custom_domain", recordAddRemoveCustomDomain)
	Register("verify_custom_domain_pending", recordVerifyCustomDomainPending)
}

// recordAddRemoveCustomDomain captures POST /core/v4/domains then
// DELETE /core/v4/domains/{id} for the throwaway domain.
// Precondition: RECORD_THROWAWAY_DOMAIN must be set and not already on the account.
func recordAddRemoveCustomDomain(ctx context.Context) error {
	domain := os.Getenv("RECORD_THROWAWAY_DOMAIN")
	if domain == "" {
		return fmt.Errorf("RECORD_THROWAWAY_DOMAIN unset")
	}
	return recordRawTool(ctx, "add_remove_custom_domain", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			added, err := protonraw.AddCustomDomain(ctx, s.Raw(ctx), domain)
			if err != nil {
				return fmt.Errorf("add domain: %w", err)
			}
			if err := protonraw.RemoveCustomDomain(ctx, s.Raw(ctx), added.ID); err != nil {
				return fmt.Errorf("remove domain %s: %w", added.ID, err)
			}
			return nil
		},
	)
}

// recordVerifyCustomDomainPending captures PUT /core/v4/domains/{id}/verify
// while the domain is in a pending-verification state (DNS not yet propagated).
//
// Precondition: the throwaway domain must already exist on the account with
// VerifyState != 1 (i.e. not yet verified). Add it via the web UI or by
// running add_remove_custom_domain and NOT running the remove step, then set
// RECORD_THROWAWAY_DOMAIN to that domain name and re-run this scenario.
func recordVerifyCustomDomainPending(ctx context.Context) error {
	domain := os.Getenv("RECORD_THROWAWAY_DOMAIN")
	if domain == "" {
		return fmt.Errorf("RECORD_THROWAWAY_DOMAIN unset")
	}
	return recordRawTool(ctx, "verify_custom_domain_pending", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			domains, err := protonraw.ListCustomDomains(ctx, s.Raw(ctx))
			if err != nil {
				return fmt.Errorf("list domains: %w", err)
			}
			var targetID string
			for _, d := range domains {
				if d.DomainName == domain {
					targetID = d.ID
					break
				}
			}
			if targetID == "" {
				return fmt.Errorf(
					"domain %q not found on account; add it first (web UI or add_remove_custom_domain)",
					domain,
				)
			}
			_, err = protonraw.VerifyCustomDomain(ctx, s.Raw(ctx), targetID)
			return err
		},
	)
}
