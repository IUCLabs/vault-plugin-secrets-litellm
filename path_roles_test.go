package litellm

import (
	"context"
	"testing"

	"github.com/hashicorp/vault/sdk/logical"
)

func TestRole_WriteReadListDelete(t *testing.T) {
	b, s := getTestBackend(t)
	ctx := context.Background()

	resp, err := b.HandleRequest(ctx, &logical.Request{
		Operation: logical.CreateOperation,
		Path:      "roles/team-a",
		Storage:   s,
		Data: map[string]interface{}{
			"models":           "gpt-4,gpt-3.5-turbo",
			"max_budget":       50.5,
			"key_alias_prefix": "team-a",
			"ttl":              "1h",
			"max_ttl":          "24h",
			"metadata":         "env=prod,team=a",
		},
	})
	if err != nil || (resp != nil && resp.IsError()) {
		t.Fatalf("write role: err=%v resp=%v", err, resp)
	}

	resp, err = b.HandleRequest(ctx, &logical.Request{
		Operation: logical.ReadOperation,
		Path:      "roles/team-a",
		Storage:   s,
	})
	if err != nil || resp == nil {
		t.Fatalf("read role: err=%v resp=%v", err, resp)
	}
	models := resp.Data["models"].([]string)
	if len(models) != 2 || models[0] != "gpt-4" {
		t.Fatalf("unexpected models: %v", models)
	}
	if resp.Data["max_budget"].(float64) != 50.5 {
		t.Fatalf("unexpected budget: %v", resp.Data["max_budget"])
	}
	if resp.Data["ttl"].(int64) != 3600 {
		t.Fatalf("unexpected ttl: %v", resp.Data["ttl"])
	}
	if resp.Data["max_ttl"].(int64) != 86400 {
		t.Fatalf("unexpected max_ttl: %v", resp.Data["max_ttl"])
	}

	// List.
	resp, err = b.HandleRequest(ctx, &logical.Request{
		Operation: logical.ListOperation,
		Path:      "roles/",
		Storage:   s,
	})
	if err != nil {
		t.Fatalf("list roles: %v", err)
	}
	keys := resp.Data["keys"].([]string)
	if len(keys) != 1 || keys[0] != "team-a" {
		t.Fatalf("unexpected role list: %v", keys)
	}

	// Delete.
	if _, err := b.HandleRequest(ctx, &logical.Request{
		Operation: logical.DeleteOperation,
		Path:      "roles/team-a",
		Storage:   s,
	}); err != nil {
		t.Fatalf("delete role: %v", err)
	}
	role, _ := b.getRole(ctx, s, "team-a")
	if role != nil {
		t.Fatal("expected role to be gone after delete")
	}
}

func TestRole_TTLGreaterThanMaxTTLRejected(t *testing.T) {
	b, s := getTestBackend(t)
	ctx := context.Background()

	resp, err := b.HandleRequest(ctx, &logical.Request{
		Operation: logical.CreateOperation,
		Path:      "roles/bad",
		Storage:   s,
		Data: map[string]interface{}{
			"ttl":     "48h",
			"max_ttl": "1h",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp == nil || !resp.IsError() {
		t.Fatal("expected error when ttl > max_ttl")
	}
}

func TestRole_UnrestrictedModels(t *testing.T) {
	b, s := getTestBackend(t)
	ctx := context.Background()

	resp, err := b.HandleRequest(ctx, &logical.Request{
		Operation: logical.CreateOperation,
		Path:      "roles/open",
		Storage:   s,
		Data:      map[string]interface{}{"max_budget": 10},
	})
	if err != nil || (resp != nil && resp.IsError()) {
		t.Fatalf("write role: err=%v resp=%v", err, resp)
	}
	role, err := b.getRole(ctx, s, "open")
	if err != nil {
		t.Fatalf("getRole: %v", err)
	}
	if len(role.Models) != 0 {
		t.Fatalf("expected no model restriction, got %v", role.Models)
	}
}
