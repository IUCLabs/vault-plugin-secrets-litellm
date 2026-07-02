package litellm

import (
	"context"
	"strings"
	"sync"

	"github.com/hashicorp/vault/sdk/framework"
	"github.com/hashicorp/vault/sdk/logical"
)

// litellmBackend is the Vault secrets backend that manages LiteLLM virtual keys.
type litellmBackend struct {
	*framework.Backend

	lock   sync.RWMutex
	client *litellmClient
}

// Factory returns a configured LiteLLM secrets backend.
func Factory(ctx context.Context, conf *logical.BackendConfig) (logical.Backend, error) {
	b := backend()
	if err := b.Setup(ctx, conf); err != nil {
		return nil, err
	}
	return b, nil
}

func backend() *litellmBackend {
	b := &litellmBackend{}

	b.Backend = &framework.Backend{
		Help:        strings.TrimSpace(backendHelp),
		BackendType: logical.TypeLogical,
		PathsSpecial: &logical.Paths{
			LocalStorage: []string{},
			SealWrapStorage: []string{
				configStoragePath,
			},
		},
		Paths: framework.PathAppend(
			[]*framework.Path{
				pathConfig(b),
				pathRole(b),
				pathRolesList(b),
				pathCredentials(b),
			},
		),
		Secrets: []*framework.Secret{
			b.litellmKey(),
		},
		Invalidate:  b.invalidate,
		WALRollback: nil,
	}

	return b
}

// reset drops the cached LiteLLM client so it is rebuilt from storage on the
// next request. Called when the config changes.
func (b *litellmBackend) reset() {
	b.lock.Lock()
	defer b.lock.Unlock()
	b.client = nil
}

// invalidate clears the cached client when the config is written from another
// cluster node.
func (b *litellmBackend) invalidate(_ context.Context, key string) {
	if key == configStoragePath {
		b.reset()
	}
}

// getClient returns a cached LiteLLM client, building one from stored config if
// necessary.
func (b *litellmBackend) getClient(ctx context.Context, s logical.Storage) (*litellmClient, error) {
	b.lock.RLock()
	if b.client != nil {
		defer b.lock.RUnlock()
		return b.client, nil
	}
	b.lock.RUnlock()

	b.lock.Lock()
	defer b.lock.Unlock()

	// Re-check after acquiring the write lock.
	if b.client != nil {
		return b.client, nil
	}

	config, err := getConfig(ctx, s)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, errBackendNotConfigured
	}

	client, err := newClient(config)
	if err != nil {
		return nil, err
	}
	b.client = client
	return b.client, nil
}

const backendHelp = `
The LiteLLM secrets backend dynamically generates LiteLLM virtual API keys
scoped to specific models, with a spending limit and a lease-bound TTL.

After mounting this backend, configure it with the connection details for the
LiteLLM proxy using the "config" path, then define one or more roles that
constrain the generated keys. Reading from the "creds/<role>" path issues a new
virtual key whose lifetime is managed by Vault: revoking the lease revokes the
key in LiteLLM.
`
