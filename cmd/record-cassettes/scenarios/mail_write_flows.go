//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/mail"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
)

func registerMailWriteFlows() {
	Register("list_labels_happy", recordListLabels)
	Register("create_draft_happy", recordCreateDraft)
}

// recordListLabels captures GetLabels for all three label types. No keyring
// unlock required — labels are plaintext metadata.
func recordListLabels(ctx context.Context) error {
	return recordReadTool(ctx, "list_labels_happy", toolsCassetteDir,
		func(c *proton.Client) error {
			if _, err := c.GetLabels(ctx, proton.LabelTypeSystem, proton.LabelTypeFolder, proton.LabelTypeLabel); err != nil {
				return fmt.Errorf("get labels: %w", err)
			}
			return nil
		})
}

// recordCreateDraft records create + update of a draft. REQUIRES a
// keyring-unlockable account (same precondition as #196): CreateDraft encrypts
// the body to the sender key. The operator records this only when such an
// account is available.
func recordCreateDraft(ctx context.Context) error {
	return recordRawTool(ctx, "create_draft_happy", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			c, err := s.Client(ctx)
			if err != nil {
				return fmt.Errorf("client: %w", err)
			}
			addrs, err := c.GetAddresses(ctx)
			if err != nil {
				return fmt.Errorf("get addresses: %w", err)
			}
			if len(addrs) == 0 {
				return fmt.Errorf("no addresses on account")
			}
			krs, err := s.Keyrings(ctx)
			if err != nil {
				return fmt.Errorf("keyrings (needs unlockable account, see #196): %w", err)
			}
			kr, err := krs.AddressKeyRing(addrs[0].ID)
			if err != nil {
				return fmt.Errorf("address keyring: %w", err)
			}
			_, err = c.CreateDraft(ctx, kr, proton.CreateDraftReq{Message: proton.DraftTemplate{
				Subject: "record draft",
				Sender:  &mail.Address{Address: addrs[0].Email},
				Body:    "recorded body",
			}})
			if err != nil {
				return fmt.Errorf("create draft: %w", err)
			}
			return nil
		})
}
