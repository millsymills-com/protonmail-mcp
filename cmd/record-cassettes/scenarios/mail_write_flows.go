//go:build recording

package scenarios

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
)

func registerMailWriteFlows() {
	Register("list_labels_happy", recordListLabels)
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
