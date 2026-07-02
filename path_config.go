package litellm

import (
	"context"
	"errors"

	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

const configStoragePath = "config"

// litellmConfig holds the connection details for the LiteLLM proxy.
type litellmConfig struct {
	BaseURL               string `json:"base_url"`
	MasterKey             string `json:"master_key"`
	RequestTimeoutSeconds int    `json:"request_timeout_seconds"`
}

func pathConfig(b *litellmBackend) *framework.Path {
	return &framework.Path{
		Pattern: "config",
		DisplayAttrs: &framework.DisplayAttributes{
			OperationPrefix: "litellm",
			OperationSuffix: "config",
		},
		Fields: map[string]*framework.FieldSchema{
			"base_url": {
				Type:        framework.TypeString,
				Description: "Base URL of the LiteLLM proxy, e.g. http://litellm:4000.",
				Required:    true,
				DisplayAttrs: &framework.DisplayAttributes{
					Name: "Base URL",
				},
			},
			"master_key": {
				Type:        framework.TypeString,
				Description: "LiteLLM master key (sk-...) used to manage virtual keys.",
				Required:    true,
				DisplayAttrs: &framework.DisplayAttributes{
					Name:      "Master Key",
					Sensitive: true,
				},
			},
			"request_timeout_seconds": {
				Type:        framework.TypeInt,
				Description: "HTTP timeout in seconds for calls to the LiteLLM proxy (default 30).",
				Default:     30,
			},
		},
		Operations: map[logical.Operation]framework.OperationHandler{
			logical.ReadOperation: &framework.PathOperation{
				Callback: b.pathConfigRead,
			},
			logical.CreateOperation: &framework.PathOperation{
				Callback: b.pathConfigWrite,
			},
			logical.UpdateOperation: &framework.PathOperation{
				Callback: b.pathConfigWrite,
			},
			logical.DeleteOperation: &framework.PathOperation{
				Callback: b.pathConfigDelete,
			},
		},
		ExistenceCheck:  b.pathConfigExistenceCheck,
		HelpSynopsis:    "Configure the connection to the LiteLLM proxy.",
		HelpDescription: "Configure the base URL and master key that the backend uses to manage LiteLLM virtual keys.",
	}
}

func (b *litellmBackend) pathConfigExistenceCheck(ctx context.Context, req *logical.Request, _ *framework.FieldData) (bool, error) {
	config, err := getConfig(ctx, req.Storage)
	if err != nil {
		return false, err
	}
	return config != nil, nil
}

func (b *litellmBackend) pathConfigRead(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	config, err := getConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}

	// The master key is deliberately not returned.
	return &logical.Response{
		Data: map[string]interface{}{
			"base_url":                config.BaseURL,
			"request_timeout_seconds": config.RequestTimeoutSeconds,
		},
	}, nil
}

func (b *litellmBackend) pathConfigWrite(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	config, err := getConfig(ctx, req.Storage)
	if err != nil {
		return nil, err
	}
	if config == nil {
		if req.Operation == logical.UpdateOperation {
			return nil, errors.New("config not found during update operation")
		}
		config = &litellmConfig{}
	}

	if v, ok := data.GetOk("base_url"); ok {
		config.BaseURL = v.(string)
	}
	if v, ok := data.GetOk("master_key"); ok {
		config.MasterKey = v.(string)
	}
	if v, ok := data.GetOk("request_timeout_seconds"); ok {
		config.RequestTimeoutSeconds = v.(int)
	} else if req.Operation == logical.CreateOperation {
		config.RequestTimeoutSeconds = data.Get("request_timeout_seconds").(int)
	}

	if config.BaseURL == "" {
		return logical.ErrorResponse("base_url is required"), nil
	}
	if config.MasterKey == "" {
		return logical.ErrorResponse("master_key is required"), nil
	}

	entry, err := logical.StorageEntryJSON(configStoragePath, config)
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(ctx, entry); err != nil {
		return nil, err
	}

	// Force the cached client to be rebuilt with the new config.
	b.reset()

	return nil, nil
}

func (b *litellmBackend) pathConfigDelete(ctx context.Context, req *logical.Request, _ *framework.FieldData) (*logical.Response, error) {
	if err := req.Storage.Delete(ctx, configStoragePath); err != nil {
		return nil, err
	}
	b.reset()
	return nil, nil
}

// getConfig reads and decodes the stored LiteLLM config, returning nil if none
// exists.
func getConfig(ctx context.Context, s logical.Storage) (*litellmConfig, error) {
	entry, err := s.Get(ctx, configStoragePath)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	config := &litellmConfig{}
	if err := entry.DecodeJSON(config); err != nil {
		return nil, err
	}
	return config, nil
}
