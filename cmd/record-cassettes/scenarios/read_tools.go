//go:build recording

package scenarios

import (
	"context"
	"fmt"

	proton "github.com/ProtonMail/go-proton-api"
)

func registerReadTools() {
	Register("whoami_happy", func(ctx context.Context) error {
		return recordReadTool(ctx, "whoami_happy", toolsCassetteDir, func(c *proton.Client) error {
			_, err := c.GetUser(ctx)
			return err
		})
	})

	Register("session_status_happy", func(ctx context.Context) error {
		return recordReadTool(ctx, "session_status_happy", toolsCassetteDir,
			func(c *proton.Client) error {
				_, err := c.GetUser(ctx)
				return err
			})
	})

	Register("list_addresses_happy", func(ctx context.Context) error {
		return recordReadTool(ctx, "list_addresses_happy", toolsCassetteDir,
			func(c *proton.Client) error {
				_, err := c.GetAddresses(ctx)
				return err
			})
	})

	Register("get_address_happy", func(ctx context.Context) error {
		return recordReadTool(ctx, "get_address_happy", toolsCassetteDir, func(c *proton.Client) error {
			addrs, err := c.GetAddresses(ctx)
			if err != nil {
				return err
			}
			if len(addrs) == 0 {
				return fmt.Errorf("test account has no addresses")
			}
			_, err = c.GetAddress(ctx, addrs[0].ID)
			return err
		})
	})

	Register("mail_settings_happy", func(ctx context.Context) error {
		return recordReadTool(ctx, "mail_settings_happy", toolsCassetteDir, func(c *proton.Client) error {
			_, err := c.GetMailSettings(ctx)
			return err
		})
	})

	Register("core_settings_happy", func(ctx context.Context) error {
		return recordReadTool(ctx, "core_settings_happy", toolsCassetteDir, func(c *proton.Client) error {
			_, err := c.GetUserSettings(ctx)
			return err
		})
	})

	Register("list_address_keys_happy", func(ctx context.Context) error {
		return recordReadTool(ctx, "list_address_keys_happy", toolsCassetteDir,
			func(c *proton.Client) error {
				addrs, err := c.GetAddresses(ctx)
				if err != nil {
					return err
				}
				if len(addrs) == 0 {
					return fmt.Errorf("no addresses to read keys for")
				}
				// Keys are embedded in the Address response — GetAddress captures them.
				_, err = c.GetAddress(ctx, addrs[0].ID)
				return err
			})
	})

	Register("search_messages_happy", func(ctx context.Context) error {
		// Page size 50 matches the consumer's default in
		// internal/tools/messages.go (proton_search_messages with an
		// empty limit). The body matcher rejects PageSize mismatches,
		// so this must stay in sync with the consumer default.
		return recordReadTool(ctx, "search_messages_happy", toolsCassetteDir,
			func(c *proton.Client) error {
				_, err := c.GetMessageMetadataPage(ctx, 0, 50, proton.MessageFilter{
					Desc: proton.Bool(true),
				})
				return err
			})
	})

	Register("get_message_happy", func(ctx context.Context) error {
		return recordReadTool(ctx, "get_message_happy", toolsCassetteDir, func(c *proton.Client) error {
			msgs, err := c.GetMessageMetadataPage(ctx, 0, 1, proton.MessageFilter{
				Desc: proton.Bool(true),
			})
			if err != nil {
				return err
			}
			if len(msgs) == 0 {
				return fmt.Errorf("test account has no messages; send one before recording")
			}
			_, err = c.GetMessage(ctx, msgs[0].ID)
			return err
		})
	})

	// list_custom_domains_happy and get_custom_domain_happy are registered in
	// custom_domain_lifecycle.go using injectors: the account's API token
	// lacks the `organization` scope that real /core/v4/domains requests
	// require, so the recorder synthesizes responses.
}
