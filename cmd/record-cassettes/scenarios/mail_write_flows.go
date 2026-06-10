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

// recordOrganizeLabel records search + star/unstar (label "10") of the first
// message. No keyring unlock needed. Star then unstar to leave the mailbox as
// found. Desc must match the filter proton_search_messages sends
// (messages.go sets Desc: proton.Bool(true)); the VCR matcher compares
// canonical JSON bodies, so a bare MessageFilter{} would never replay.
func recordOrganizeLabel(ctx context.Context) error {
	return recordRawTool(ctx, "organize_label_happy", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			c, err := s.Client(ctx)
			if err != nil {
				return fmt.Errorf("client: %w", err)
			}
			meta, err := c.GetMessageMetadataPage(ctx, 0, 1, proton.MessageFilter{
				Desc: proton.Bool(true),
			})
			if err != nil {
				return fmt.Errorf("metadata page: %w", err)
			}
			if len(meta) == 0 {
				return fmt.Errorf("no messages to label; account inbox is empty")
			}
			id := meta[0].ID
			if err := c.LabelMessages(ctx, []string{id}, proton.StarredLabel); err != nil {
				return fmt.Errorf("label: %w", err)
			}
			if err := c.UnlabelMessages(ctx, []string{id}, proton.StarredLabel); err != nil {
				return fmt.Errorf("unlabel: %w", err)
			}
			return nil
		})
}

func registerMailWriteFlows() {
	Register("list_labels_happy", recordListLabels)
	Register("create_draft_happy", recordCreateDraft)
	Register("organize_label_happy", recordOrganizeLabel)
	Register("delete_messages_happy", recordDeleteMessages)
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

// recordDeleteMessages creates a throwaway draft (needs an unlockable account,
// like create_draft, see #196) then permanently deletes it, so the trace
// exercises the expunge without destroying a real message. The drafts search
// between create and delete is what the cassette test replays to find the ID;
// its filter must stay byte-identical to what proton_search_messages sends for
// label_id "8" / limit 1 (LabelID + Desc:true, page 0, size 1).
func recordDeleteMessages(ctx context.Context) error {
	return recordRawTool(ctx, "delete_messages_happy", toolsCassetteDir,
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
			draft, err := c.CreateDraft(ctx, kr, proton.CreateDraftReq{Message: proton.DraftTemplate{
				Subject: "throwaway", Sender: &mail.Address{Address: addrs[0].Email}, Body: "delete me",
			}})
			if err != nil {
				return fmt.Errorf("create throwaway draft: %w", err)
			}
			if _, err := c.GetMessageMetadataPage(ctx, 0, 1, proton.MessageFilter{
				LabelID: proton.DraftsLabel,
				Desc:    proton.Bool(true),
			}); err != nil {
				return fmt.Errorf("search drafts: %w", err)
			}
			if err := c.DeleteMessage(ctx, draft.ID); err != nil {
				return fmt.Errorf("delete: %w", err)
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
