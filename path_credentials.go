package litellm

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/go-uuid"
	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

func pathCredentials(b *litellmBackend) *framework.Path {
	return &framework.Path{
		Pattern: "creds/" + framework.GenericNameRegex("name"),
		DisplayAttrs: &framework.DisplayAttributes{
			OperationPrefix: "litellm",
			OperationSuffix: "credentials",
		},
		Fields: map[string]*framework.FieldSchema{
			"name": {
				Type:        framework.TypeLowerCaseString,
				Description: "Name of the role to generate a key for.",
				Required:    true,
			},
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation: &framework.PathOperation{
				Callback: b.pathCredentialsRead,
			},
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.pathCredentialsRead,
			},
		},
		HelpSynopsis:    "Generate a LiteLLM virtual key from a role.",
		HelpDescription: "Reading from this path generates a new LiteLLM virtual key scoped by the named role's models, budget, and TTL.",
	}
}

func (b *litellmBackend) pathCredentialsRead(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	roleName := data.Get("name").(string)

	role, err := b.getRole(ctx, req.Storage, roleName)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return logical.ErrorResponse("role %q does not exist", roleName), nil
	}

	return b.createKey(ctx, req, roleName, role)
}

// createKey issues a new LiteLLM virtual key for the given role and wraps it in
// a Vault lease.
func (b *litellmBackend) createKey(ctx context.Context, req *logical.Request, roleName string, role *litellmRole) (*logical.Response, error) {
	client, err := b.getClient(ctx, req.Storage)
	if err != nil {
		return nil, err
	}

	// Resolve the lease TTL against the role and mount/system limits.
	ttl, maxTTL := b.resolveTTLs(role)

	suffix, err := uuid.GenerateUUID()
	if err != nil {
		return nil, fmt.Errorf("generating key alias suffix: %w", err)
	}
	alias := fmt.Sprintf("%s-%s-%s", role.KeyAliasPrefix, roleName, suffix[:8])

	genReq := generateKeyRequest{
		Models:   role.Models,
		KeyAlias: alias,
		Metadata: role.Metadata,
	}
	if role.MaxBudget > 0 {
		budget := role.MaxBudget
		genReq.MaxBudget = &budget
	}
	if ttl > 0 {
		genReq.Duration = durationToLiteLLM(ttl)
	}

	key, err := client.GenerateKey(ctx, genReq)
	if err != nil {
		return nil, fmt.Errorf("generating litellm key: %w", err)
	}

	internal := map[string]interface{}{
		"key":       key.Key,
		"key_alias": alias,
		"role":      roleName,
	}
	external := map[string]interface{}{
		"key":        key.Key,
		"key_alias":  alias,
		"models":     role.Models,
		"max_budget": role.MaxBudget,
		"role":       roleName,
	}

	resp := b.Secret(litellmKeyType).Response(external, internal)
	resp.Secret.TTL = ttl
	resp.Secret.MaxTTL = maxTTL
	if ttl > 0 {
		resp.Secret.Renewable = true
	}

	return resp, nil
}

// resolveTTLs clamps the role's TTL/MaxTTL against the mount and system limits.
func (b *litellmBackend) resolveTTLs(role *litellmRole) (ttl, maxTTL time.Duration) {
	sysMaxTTL := b.System().MaxLeaseTTL()

	maxTTL = role.MaxTTL
	if maxTTL <= 0 || maxTTL > sysMaxTTL {
		maxTTL = sysMaxTTL
	}

	ttl = role.TTL
	if ttl <= 0 {
		ttl = b.System().DefaultLeaseTTL()
	}
	if ttl > maxTTL {
		ttl = maxTTL
	}
	return ttl, maxTTL
}

// durationToLiteLLM converts a Go duration to a LiteLLM duration string. LiteLLM
// accepts an integer with an s/m/h/d suffix; seconds is always representable.
func durationToLiteLLM(d time.Duration) string {
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
