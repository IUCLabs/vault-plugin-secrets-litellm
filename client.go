package litellm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// errBackendNotConfigured is returned when an operation needs the LiteLLM
// connection config but none has been written yet.
var errBackendNotConfigured = errors.New("litellm backend not configured: write connection details to config/ first")

const defaultHTTPTimeout = 30 * time.Second

// litellmClient talks to a LiteLLM proxy's key-management API using the master
// key for authentication.
type litellmClient struct {
	baseURL    string
	masterKey  string
	httpClient *http.Client
}

func newClient(config *litellmConfig) (*litellmClient, error) {
	if config == nil {
		return nil, errors.New("litellm client configuration is nil")
	}
	if config.BaseURL == "" {
		return nil, errors.New("base_url is required")
	}
	if config.MasterKey == "" {
		return nil, errors.New("master_key is required")
	}

	timeout := defaultHTTPTimeout
	if config.RequestTimeoutSeconds > 0 {
		timeout = time.Duration(config.RequestTimeoutSeconds) * time.Second
	}

	return &litellmClient{
		baseURL:    strings.TrimRight(config.BaseURL, "/"),
		masterKey:  config.MasterKey,
		httpClient: &http.Client{Timeout: timeout},
	}, nil
}

// generateKeyRequest is the payload for POST /key/generate.
type generateKeyRequest struct {
	Models    []string               `json:"models,omitempty"`
	MaxBudget *float64               `json:"max_budget,omitempty"`
	Duration  string                 `json:"duration,omitempty"`
	KeyAlias  string                 `json:"key_alias,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// GenerateKeyResponse is the subset of the /key/generate response we consume.
type GenerateKeyResponse struct {
	Key       string   `json:"key"`
	KeyName   string   `json:"key_name"`
	TokenID   string   `json:"token_id"`
	Expires   string   `json:"expires"`
	MaxBudget *float64 `json:"max_budget"`
	Models    []string `json:"models"`
}

// updateKeyRequest is the payload for POST /key/update.
type updateKeyRequest struct {
	Key       string   `json:"key"`
	Duration  string   `json:"duration,omitempty"`
	MaxBudget *float64 `json:"max_budget,omitempty"`
	Models    []string `json:"models,omitempty"`
}

// KeyInfoResponse is the subset of the /key/info response we consume.
type KeyInfoResponse struct {
	Key  string `json:"key"`
	Info struct {
		Models    []string `json:"models"`
		MaxBudget *float64 `json:"max_budget"`
		Spend     float64  `json:"spend"`
		Expires   string   `json:"expires"`
		KeyName   string   `json:"key_name"`
	} `json:"info"`
}

// GenerateKey creates a new virtual key on the LiteLLM proxy.
func (c *litellmClient) GenerateKey(ctx context.Context, req generateKeyRequest) (*GenerateKeyResponse, error) {
	var out GenerateKeyResponse
	if err := c.do(ctx, http.MethodPost, "/key/generate", req, &out); err != nil {
		return nil, err
	}
	if out.Key == "" {
		return nil, errors.New("litellm returned an empty key")
	}
	return &out, nil
}

// UpdateKey changes the TTL, budget, or model scope of an existing key.
func (c *litellmClient) UpdateKey(ctx context.Context, req updateKeyRequest) error {
	if req.Key == "" {
		return errors.New("key is required to update")
	}
	return c.do(ctx, http.MethodPost, "/key/update", req, nil)
}

// DeleteKey revokes a virtual key on the LiteLLM proxy.
func (c *litellmClient) DeleteKey(ctx context.Context, key string) error {
	if key == "" {
		return errors.New("key is required to delete")
	}
	body := map[string]interface{}{"keys": []string{key}}
	return c.do(ctx, http.MethodPost, "/key/delete", body, nil)
}

// KeyInfo fetches metadata about an existing key.
func (c *litellmClient) KeyInfo(ctx context.Context, key string) (*KeyInfoResponse, error) {
	path := "/key/info?" + url.Values{"key": {key}}.Encode()
	var out KeyInfoResponse
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// do performs an authenticated HTTP request against the LiteLLM proxy and
// decodes the JSON response into out (when non-nil).
func (c *litellmClient) do(ctx context.Context, method, path string, payload, out interface{}) error {
	var bodyReader io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("encoding request body: %w", err)
		}
		bodyReader = bytes.NewReader(raw)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.masterKey)
	req.Header.Set("Accept", "application/json")
	if bodyReader != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("calling litellm %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("litellm %s %s returned %d: %s", method, path, resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	if out == nil {
		return nil
	}
	if err := json.Unmarshal(respBody, out); err != nil {
		return fmt.Errorf("decoding litellm response: %w", err)
	}
	return nil
}
