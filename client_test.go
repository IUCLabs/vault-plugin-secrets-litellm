package litellm

import (
	"context"
	"testing"
)

func TestClient_GenerateKey(t *testing.T) {
	m := newMockLiteLLM(t)
	client, err := newClient(&litellmConfig{BaseURL: m.server.URL, MasterKey: m.masterKey})
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}

	budget := 25.0
	resp, err := client.GenerateKey(context.Background(), generateKeyRequest{
		Models:    []string{"gpt-4"},
		MaxBudget: &budget,
		Duration:  "3600s",
		KeyAlias:  "vault-test",
	})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if resp.Key == "" {
		t.Fatal("expected a non-empty key")
	}
	if m.keyCount() != 1 {
		t.Fatalf("expected 1 key on server, got %d", m.keyCount())
	}
	if got := m.lastRequest["key_alias"]; got != "vault-test" {
		t.Fatalf("expected alias forwarded, got %v", got)
	}
	if got := m.lastRequest["max_budget"]; got != 25.0 {
		t.Fatalf("expected budget forwarded, got %v", got)
	}
}

func TestClient_DeleteKey(t *testing.T) {
	m := newMockLiteLLM(t)
	client, _ := newClient(&litellmConfig{BaseURL: m.server.URL, MasterKey: m.masterKey})

	resp, err := client.GenerateKey(context.Background(), generateKeyRequest{KeyAlias: "vault-test"})
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if err := client.DeleteKey(context.Background(), resp.Key); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}
	if m.keyCount() != 0 {
		t.Fatalf("expected 0 keys after delete, got %d", m.keyCount())
	}
}

func TestClient_UpdateKey(t *testing.T) {
	m := newMockLiteLLM(t)
	client, _ := newClient(&litellmConfig{BaseURL: m.server.URL, MasterKey: m.masterKey})

	resp, _ := client.GenerateKey(context.Background(), generateKeyRequest{KeyAlias: "vault-test"})
	if err := client.UpdateKey(context.Background(), updateKeyRequest{Key: resp.Key, Duration: "7200s"}); err != nil {
		t.Fatalf("UpdateKey: %v", err)
	}
	if got := m.keys[resp.Key].Duration; got != "7200s" {
		t.Fatalf("expected duration updated to 7200s, got %q", got)
	}
}

func TestClient_AuthFailure(t *testing.T) {
	m := newMockLiteLLM(t)
	client, _ := newClient(&litellmConfig{BaseURL: m.server.URL, MasterKey: "wrong-key"})

	_, err := client.GenerateKey(context.Background(), generateKeyRequest{})
	if err == nil {
		t.Fatal("expected an auth error, got nil")
	}
}

func TestClient_ServerError(t *testing.T) {
	m := newMockLiteLLM(t)
	m.generateErr = true
	client, _ := newClient(&litellmConfig{BaseURL: m.server.URL, MasterKey: m.masterKey})

	if _, err := client.GenerateKey(context.Background(), generateKeyRequest{}); err == nil {
		t.Fatal("expected a server error, got nil")
	}
}

func TestNewClient_Validation(t *testing.T) {
	cases := []struct {
		name    string
		config  *litellmConfig
		wantErr bool
	}{
		{"nil config", nil, true},
		{"missing base_url", &litellmConfig{MasterKey: "sk"}, true},
		{"missing master_key", &litellmConfig{BaseURL: "http://x"}, true},
		{"valid", &litellmConfig{BaseURL: "http://x", MasterKey: "sk"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := newClient(tc.config)
			if (err != nil) != tc.wantErr {
				t.Fatalf("newClient err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestNewClient_TrimsTrailingSlash(t *testing.T) {
	client, err := newClient(&litellmConfig{BaseURL: "http://localhost:4000/", MasterKey: "sk"})
	if err != nil {
		t.Fatalf("newClient: %v", err)
	}
	if client.baseURL != "http://localhost:4000" {
		t.Fatalf("expected trailing slash trimmed, got %q", client.baseURL)
	}
}

func TestDurationToLiteLLM(t *testing.T) {
	if got := durationToLiteLLM(90); got != "0s" {
		t.Fatalf("sub-second duration should floor to 0s, got %q", got)
	}
}
