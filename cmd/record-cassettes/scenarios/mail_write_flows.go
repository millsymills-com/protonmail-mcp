//go:build recording

package scenarios

import (
	"context"
	"fmt"
	"net/mail"

	"github.com/ProtonMail/gluon/rfc822"
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

// recordCreateDraft records creating a draft. REQUIRES a keyring-unlockable
// account (same precondition as #196): CreateDraft encrypts the body to the
// sender key. The operator records this only when such an account is
// available.
//
// INVARIANT: the CreateDraft request below must stay byte-identical to the
// one TestCreateDraftHappyCassette makes proton_create_draft send — the VCR
// body matcher compares non-redacted JSON fields (Subject, ToList, MIMEType)
// exactly, so any divergence means no interaction matches on replay. The
// sender selection mirrors resolveSender (enabled + send-allowed, lowest
// Order) and the template mirrors draftTemplate for the test's inputs.
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
			var sender *proton.Address
			for i := range addrs {
				a := &addrs[i]
				if a.Status != proton.AddressStatusEnabled || !bool(a.Send) {
					continue
				}
				if sender == nil || a.Order < sender.Order {
					sender = a
				}
			}
			if sender == nil {
				return fmt.Errorf("no enabled sending address on account")
			}
			krs, err := s.Keyrings(ctx)
			if err != nil {
				return fmt.Errorf("keyrings (needs unlockable account, see #196): %w", err)
			}
			kr, err := krs.AddressKeyRing(sender.ID)
			if err != nil {
				return fmt.Errorf("address keyring: %w", err)
			}
			_, err = c.CreateDraft(ctx, kr, proton.CreateDraftReq{Message: proton.DraftTemplate{
				Subject:  "Hello from the agent",
				Sender:   &mail.Address{Name: sender.DisplayName, Address: sender.Email},
				ToList:   []*mail.Address{{Address: "recipient@example.test"}},
				Body:     "This is a draft body.",
				MIMEType: rfc822.TextPlain,
			}})
			if err != nil {
				return fmt.Errorf("create draft: %w", err)
			}
			return nil
		})
}
