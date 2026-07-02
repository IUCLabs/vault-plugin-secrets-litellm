package litellm

import (
	"context"
	"errors"
	"fmt"

	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

// litellmKeyType is the identifier for the dynamic secret produced by this
// backend.
const litellmKeyType = "litellm_key"

func (b *litellmBackend) litellmKey() *framework.Secret {
	return &framework.Secret{
		Type: litellmKeyType,
		Fields: map[string]*framework.FieldSchema{
			"key": {
				Type:        framework.TypeString,
				Description: "The LiteLLM virtual key.",
			},
			"key_alias": {
				Type:        framework.TypeString,
				Description: "The alias assigned to the virtual key.",
			},
		},
		Revoke: b.keyRevoke,
		Renew:  b.keyRenew,
	}
}

// keyRevoke deletes the virtual key from the LiteLLM proxy when the lease is
// revoked.
func (b *litellmBackend) keyRevoke(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	rawKey, ok := req.Secret.InternalData["key"]
	if !ok {
		return nil, errors.New("secret is missing internal key data")
	}
	key, ok := rawKey.(string)
	if !ok {
		return nil, errors.New("secret internal key data is not a string")
	}

	client, err := b.getClient(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	if err := client.DeleteKey(ctx, key); err != nil {
		return nil, fmt.Errorf("revoking litellm key: %w", err)
	}
	return nil, nil
}

// keyRenew extends the lease and, when a TTL is set, pushes the new expiry to
// LiteLLM so the virtual key and the Vault lease stay in sync.
func (b *litellmBackend) keyRenew(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	rawRole, ok := req.Secret.InternalData["role"]
	if !ok {
		return nil, errors.New("secret is missing internal role data")
	}
	roleName, ok := rawRole.(string)
	if !ok {
		return nil, errors.New("secret internal role data is not a string")
	}

	role, err := b.getRole(ctx, req.Storage, roleName)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, fmt.Errorf("role %q no longer exists; cannot renew", roleName)
	}

	resp := &logical.Response{Secret: req.Secret}
	if role.TTL > 0 {
		resp.Secret.TTL = role.TTL
	}
	if role.MaxTTL > 0 {
		resp.Secret.MaxTTL = role.MaxTTL
	}

	// Best-effort: extend the key's own expiry in LiteLLM to match the renewed
	// lease. A failure here should not fail the renew, since the authoritative
	// lifetime is the Vault lease.
	if role.TTL > 0 {
		if rawKey, ok := req.Secret.InternalData["key"].(string); ok {
			client, err := b.getClient(ctx, req.Storage)
			if err == nil {
				_ = client.UpdateKey(ctx, updateKeyRequest{
					Key:      rawKey,
					Duration: durationToLiteLLM(role.TTL),
				})
			}
		}
	}

	return resp, nil
}
