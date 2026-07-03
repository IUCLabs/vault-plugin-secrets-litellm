package litellm

import (
	"context"
	"testing"
	"time"

	"github.com/hashicorp/vault/sdk/logical"
)

func createRole(t *testing.T, b *litellmBackend, s logical.Storage, name string, data map[string]interface{}) {
	t.Helper()
	resp, err := b.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.CreateOperation,
		Path:      "roles/" + name,
		Storage:   s,
		Data:      data,
	})
	if err != nil || (resp != nil && resp.IsError()) {
		t.Fatalf("create role %s: err=%v resp=%v", name, err, resp)
	}
}

func TestCredentials_GenerateAndRevoke(t *testing.T) {
	b, s := getTestBackend(t)
	ctx := context.Background()
	m := newMockLiteLLM(t)

	writeTestConfig(t, b, s, m.server.URL, m.masterKey)
	createRole(t, b, s, "team-a", map[string]interface{}{
		"models":     "gpt-4,gpt-3.5-turbo",
		"max_budget": 50.0,
		"ttl":        "1h",
		"max_ttl":    "24h",
	})

	// Generate.
	resp, err := b.HandleRequest(ctx, &logical.Request{
		Operation: logical.ReadOperation,
		Path:      "creds/team-a",
		Storage:   s,
	})
	if err != nil || resp == nil {
		t.Fatalf("generate creds: err=%v resp=%v", err, resp)
	}
	if resp.Secret == nil {
		t.Fatal("expected a secret in the response")
	}
	key, _ := resp.Data["key"].(string)
	if key == "" {
		t.Fatal("expected a non-empty key in response data")
	}
	if resp.Secret.TTL != time.Hour {
		t.Fatalf("expected TTL 1h, got %s", resp.Secret.TTL)
	}
	if !resp.Secret.Renewable {
		t.Fatal("expected the lease to be renewable")
	}
	if m.keyCount() != 1 {
		t.Fatalf("expected 1 key on server, got %d", m.keyCount())
	}

	// The scope (models + budget) must have been forwarded to LiteLLM.
	if got := m.lastRequest["max_budget"]; got != 50.0 {
		t.Fatalf("expected budget forwarded, got %v", got)
	}
	if models, ok := m.lastRequest["models"].([]interface{}); !ok || len(models) != 2 {
		t.Fatalf("expected 2 models forwarded, got %v", m.lastRequest["models"])
	}

	// Revoke via the secret's revoke callback.
	revokeReq := &logical.Request{
		Operation: logical.RevokeOperation,
		Path:      "creds/team-a",
		Storage:   s,
		Secret:    resp.Secret,
	}
	if _, err := b.HandleRequest(ctx, revokeReq); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if m.keyCount() != 0 {
		t.Fatalf("expected key deleted on revoke, got %d keys", m.keyCount())
	}
}

func TestCredentials_UnknownRole(t *testing.T) {
	b, s := getTestBackend(t)
	m := newMockLiteLLM(t)
	writeTestConfig(t, b, s, m.server.URL, m.masterKey)

	resp, err := b.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.ReadOperation,
		Path:      "creds/nope",
		Storage:   s,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || !resp.IsError() {
		t.Fatal("expected an error response for an unknown role")
	}
}

func TestCredentials_NotConfigured(t *testing.T) {
	b, s := getTestBackend(t)
	createRole(t, b, s, "team-a", map[string]interface{}{"ttl": "1h"})

	_, err := b.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.ReadOperation,
		Path:      "creds/team-a",
		Storage:   s,
	})
	if err == nil {
		t.Fatal("expected an error when backend is not configured")
	}
}

func TestCredentials_TTLClampedToMaxTTL(t *testing.T) {
	b, s := getTestBackend(t)
	m := newMockLiteLLM(t)
	writeTestConfig(t, b, s, m.server.URL, m.masterKey)

	// TTL exceeds max_ttl in resolveTTLs (role stores equal here; test the
	// clamp against the mount max via a very large ttl with small max_ttl is
	// rejected at write time, so instead verify default-lease fallback).
	createRole(t, b, s, "nolease", map[string]interface{}{"max_ttl": "2h"})

	resp, err := b.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.ReadOperation,
		Path:      "creds/nolease",
		Storage:   s,
	})
	if err != nil || resp == nil {
		t.Fatalf("generate creds: err=%v resp=%v", err, resp)
	}
	// With no role TTL, the mount default lease TTL is used, clamped by max_ttl.
	if resp.Secret.TTL <= 0 || resp.Secret.TTL > 2*time.Hour {
		t.Fatalf("expected TTL within (0, 2h], got %s", resp.Secret.TTL)
	}
}

func TestCredentials_Renew(t *testing.T) {
	b, s := getTestBackend(t)
	ctx := context.Background()
	m := newMockLiteLLM(t)
	writeTestConfig(t, b, s, m.server.URL, m.masterKey)
	createRole(t, b, s, "team-a", map[string]interface{}{"ttl": "1h", "max_ttl": "24h"})

	resp, err := b.HandleRequest(ctx, &logical.Request{
		Operation: logical.ReadOperation,
		Path:      "creds/team-a",
		Storage:   s,
	})
	if err != nil || resp == nil {
		t.Fatalf("generate creds: err=%v resp=%v", err, resp)
	}

	renewReq := &logical.Request{
		Operation: logical.RenewOperation,
		Path:      "creds/team-a",
		Storage:   s,
		Secret:    resp.Secret,
	}
	renewResp, err := b.HandleRequest(ctx, renewReq)
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if renewResp == nil || renewResp.Secret == nil {
		t.Fatal("expected a secret in the renew response")
	}
	if renewResp.Secret.TTL != time.Hour {
		t.Fatalf("expected renewed TTL 1h, got %s", renewResp.Secret.TTL)
	}
}
