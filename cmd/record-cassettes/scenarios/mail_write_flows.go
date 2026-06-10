//go:build recording

package scenarios

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
	"github.com/millsymills-com/protonmail-mcp/internal/session"
)

func registerMailWriteFlows() {
	Register("list_labels_happy", recordListLabels)
}

// recordListLabels captures GetLabels for all three label types. No keyring
// unlock required — labels are plaintext metadata.
func recordListLabels(ctx context.Context) error {
	return recordRawTool(ctx, "list_labels_happy", toolsCassetteDir,
		func(ctx context.Context, s *session.Session) error {
			c, err := s.Client(ctx)
			if err != nil {
				return fmt.Errorf("client: %w", err)
			}
			if _, err := c.GetLabels(ctx, proton.LabelTypeSystem, proton.LabelTypeFolder, proton.LabelTypeLabel); err != nil {
				return fmt.Errorf("get labels: %w", err)
			}
			return nil
		})
}
