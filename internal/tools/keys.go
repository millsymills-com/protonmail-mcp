package tools

import (
	"context"

	"github.com/ProtonMail/gopenpgp/v2/crypto"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/millsmillsymills/protonmail-mcp/internal/proterr"
)

// keyDTO exposes the public-facing properties of an address key. We derive
// fingerprint and armored public key from the binary PrivateKey bytes via
// gopenpgp; this requires no passphrase / unlocked keyring (only public
// material is read).
type keyDTO struct {
	ID          string `json:"id"`
	Fingerprint string `json:"fingerprint,omitempty"`
	PublicKey   string `json:"public_key_armored,omitempty"`
	Primary     bool   `json:"primary"`
	Active      bool   `json:"active"`
	Flags       int    `json:"flags"`
}

type listKeysIn struct {
	AddressID string `json:"address_id"`
}
type listKeysOut struct {
	Keys []keyDTO `json:"keys"`
}

func registerKeys(server *mcp.Server, d Deps) {
	mcp.AddTool(server, &mcp.Tool{
		Name:        "proton_list_address_keys",
		Description: "Lists encryption keys for an address (id, fingerprint, primary flag, armored public key).",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listKeysIn) (*mcp.CallToolResult, listKeysOut, error) {
		c, fail := clientOrFail(ctx, d)
		if fail != nil {
			return fail, listKeysOut{}, nil
		}
		addr, err := c.GetAddress(ctx, in.AddressID)
		if err != nil {
			return failure(proterr.Map(err)), listKeysOut{}, nil
		}
		out := make([]keyDTO, len(addr.Keys))
		for i, k := range addr.Keys {
			dto := keyDTO{
				ID:      k.ID,
				Primary: bool(k.Primary),
				Active:  bool(k.Active),
				Flags:   int(k.Flags),
			}
			// Best-effort: derive fingerprint and armored public key from the
			// (locked) private-key blob. Failure is non-fatal — we still
			// return the key entry with metadata.
			if pk, perr := crypto.NewKey(k.PrivateKey); perr == nil && pk != nil {
				dto.Fingerprint = pk.GetFingerprint()
				if armored, aerr := pk.GetArmoredPublicKey(); aerr == nil {
					dto.PublicKey = armored
				}
			}
			out[i] = dto
		}
		return nil, listKeysOut{Keys: out}, nil
	})

	// NOTE: Key write operations are deferred. go-proton-api exposes
	// MakeAddressKeyPrimary / DeleteAddressKey / CreateAddressKey, but they
	// require a signed KeyList parameter — building that requires unlocking
	// the user's keyring with the account passphrase, which is v1.5
	// territory. MakeAndCreateAddressKey from the original plan does not
	// exist in this version of go-proton-api.
}
