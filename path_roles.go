package litellm

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

const roleStoragePrefix = "role/"

// litellmRole constrains the virtual keys generated from the creds/<name> path.
type litellmRole struct {
	// Models is the list of model names a generated key is allowed to call.
	// An empty list means the key is not restricted to specific models.
	Models []string `json:"models"`
	// MaxBudget is the spending limit (in the proxy's currency units) applied
	// to each generated key. Zero means unlimited.
	MaxBudget float64 `json:"max_budget"`
	// KeyAliasPrefix is prepended to the auto-generated alias for each key.
	KeyAliasPrefix string `json:"key_alias_prefix"`
	// Metadata is attached to each generated key in LiteLLM.
	Metadata map[string]interface{} `json:"metadata"`
	// TTL is the default lease duration for keys issued from this role.
	TTL time.Duration `json:"ttl"`
	// MaxTTL is the maximum lease duration for keys issued from this role.
	MaxTTL time.Duration `json:"max_ttl"`
}

func (r *litellmRole) toResponseData() map[string]interface{} {
	return map[string]interface{}{
		"models":           r.Models,
		"max_budget":       r.MaxBudget,
		"key_alias_prefix": r.KeyAliasPrefix,
		"metadata":         r.Metadata,
		"ttl":              int64(r.TTL.Seconds()),
		"max_ttl":          int64(r.MaxTTL.Seconds()),
	}
}

func pathRole(b *litellmBackend) *framework.Path {
	return &framework.Path{
		Pattern: "roles/" + framework.GenericNameRegex("name"),
		DisplayAttrs: &framework.DisplayAttributes{
			OperationPrefix: "litellm",
			OperationSuffix: "role",
		},
		Fields: map[string]*framework.FieldSchema{
			"name": {
				Type:        framework.TypeLowerCaseString,
				Description: "Name of the role.",
				Required:    true,
			},
			"models": {
				Type:        framework.TypeCommaStringSlice,
				Description: "Comma-separated list of models a generated key may access. Empty means unrestricted.",
			},
			"max_budget": {
				Type:        framework.TypeFloat,
				Description: "Spending limit applied to each generated key. 0 means unlimited.",
			},
			"key_alias_prefix": {
				Type:        framework.TypeString,
				Description: "Prefix for the auto-generated key alias.",
				Default:     "vault",
			},
			"metadata": {
				Type:        framework.TypeKVPairs,
				Description: "Arbitrary key=value metadata attached to each generated key.",
			},
			"ttl": {
				Type:        framework.TypeDurationSecond,
				Description: "Default lease TTL for keys generated from this role.",
			},
			"max_ttl": {
				Type:        framework.TypeDurationSecond,
				Description: "Maximum lease TTL for keys generated from this role.",
			},
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation: &framework.PathOperation{
				Callback: b.pathRoleRead,
			},
			logical.CreateOperation: &framework.PathOperation{
				Callback: b.pathRoleWrite,
			},
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.pathRoleWrite,
			},
			logical.DeleteOperation: &framework.PathOperation{
				Callback: b.pathRoleDelete,
			},
		},
		ExistenceCheck:  b.pathRoleExistenceCheck,
		HelpSynopsis:    "Manage roles that constrain generated LiteLLM keys.",
		HelpDescription: "Roles define the allowed models, spending limit, and TTLs applied to virtual keys issued from creds/<name>.",
	}
}

func pathRolesList(b *litellmBackend) *framework.Path {
	return &framework.Path{
		Pattern: "roles/?$",
		DisplayAttrs: &framework.DisplayAttributes{
			OperationPrefix: "litellm",
			OperationSuffix: "roles",
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ListOperation: &framework.PathOperation{
				Callback: b.pathRolesList,
			},
		},
		HelpSynopsis:    "List the configured roles.",
		HelpDescription: "List the roles configured on this LiteLLM backend.",
	}
}

func (b *litellmBackend) pathRoleExistenceCheck(ctx context.Context, req *logical.Request, data *framework.FieldData) (bool, error) {
	role, err := b.getRole(ctx, req.Storage, data.Get("name").(string))
	if err != nil {
		return false, err
	}
	return role != nil, nil
}

func (b *litellmBackend) pathRolesList(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	entries, err := req.Storage.List(ctx, roleStoragePrefix)
	if err != nil {
		return nil, err
	}
	return logical.ListResponse(entries), nil
}

func (b *litellmBackend) pathRoleRead(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	role, err := b.getRole(ctx, req.Storage, data.Get("name").(string))
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, nil
	}
	return &logical.Response{Data: role.toResponseData()}, nil
}

func (b *litellmBackend) pathRoleWrite(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	name := data.Get("name").(string)
	if name == "" {
		return logical.ErrorResponse("role name is required"), nil
	}

	role, err := b.getRole(ctx, req.Storage, name)
	if err != nil {
		return nil, err
	}
	if role == nil {
		role = &litellmRole{KeyAliasPrefix: "vault"}
	}

	if v, ok := data.GetOk("models"); ok {
		role.Models = v.([]string)
	}
	if v, ok := data.GetOk("max_budget"); ok {
		role.MaxBudget = v.(float64)
	}
	if v, ok := data.GetOk("key_alias_prefix"); ok {
		role.KeyAliasPrefix = v.(string)
	}
	if v, ok := data.GetOk("metadata"); ok {
		md := make(map[string]interface{})
		for k, val := range v.(map[string]string) {
			md[k] = val
		}
		role.Metadata = md
	}
	if v, ok := data.GetOk("ttl"); ok {
		role.TTL = time.Duration(v.(int)) * time.Second
	}
	if v, ok := data.GetOk("max_ttl"); ok {
		role.MaxTTL = time.Duration(v.(int)) * time.Second
	}

	if role.MaxBudget < 0 {
		return logical.ErrorResponse("max_budget must not be negative"), nil
	}
	if role.MaxTTL != 0 && role.TTL > role.MaxTTL {
		return logical.ErrorResponse("ttl must not be greater than max_ttl"), nil
	}

	if err := setRole(ctx, req.Storage, name, role); err != nil {
		return nil, err
	}
	return nil, nil
}

func (b *litellmBackend) pathRoleDelete(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	if err := req.Storage.Delete(ctx, roleStoragePrefix+data.Get("name").(string)); err != nil {
		return nil, fmt.Errorf("error deleting litellm role: %w", err)
	}
	return nil, nil
}

func (b *litellmBackend) getRole(ctx context.Context, s logical.Storage, name string) (*litellmRole, error) {
	if name == "" {
		return nil, fmt.Errorf("missing role name")
	}
	entry, err := s.Get(ctx, roleStoragePrefix+name)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}
	role := &litellmRole{}
	if err := entry.DecodeJSON(role); err != nil {
		return nil, err
	}
	return role, nil
}

func setRole(ctx context.Context, s logical.Storage, name string, role *litellmRole) error {
	entry, err := logical.StorageEntryJSON(roleStoragePrefix+name, role)
	if err != nil {
		return err
	}
	if entry == nil {
		return fmt.Errorf("failed to create storage entry for role %q", name)
	}
	return s.Put(ctx, entry)
}
