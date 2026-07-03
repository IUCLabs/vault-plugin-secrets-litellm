package litellm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"

	"github.com/hashicorp/vault/sdk/logical"
)

// getTestBackend returns a configured backend backed by in-memory storage.
func getTestBackend(t *testing.T) (*litellmBackend, logical.Storage) {
	t.Helper()

	config := logical.TestBackendConfig()
	config.StorageView = &logical.InmemStorage{}
	config.System = logical.TestSystemView()

	b, err := Factory(context.Background(), config)
	if err != nil {
		t.Fatalf("unexpected error creating backend: %v", err)
	}
	return b.(*litellmBackend), config.StorageView
}

// mockLiteLLM is an in-memory fake of the LiteLLM key-management API.
type mockLiteLLM struct {
	server *httptest.Server

	mu        sync.Mutex
	keys      map[string]mockKey // key value -> key
	counter   int
	masterKey string

	// generateErr, when set, makes /key/generate return 500.
	generateErr bool
	lastRequest map[string]interface{}
}

type mockKey struct {
	Alias     string
	Models    []string
	MaxBudget *float64
	Duration  string
}

func newMockLiteLLM(t *testing.T) *mockLiteLLM {
	t.Helper()
	m := &mockLiteLLM{
		keys:      make(map[string]mockKey),
		masterKey: "sk-master-1234",
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/key/generate", m.handleGenerate)
	mux.HandleFunc("/key/delete", m.handleDelete)
	mux.HandleFunc("/key/update", m.handleUpdate)
	mux.HandleFunc("/key/info", m.handleInfo)

	m.server = httptest.NewServer(m.authMiddleware(mux))
	t.Cleanup(m.server.Close)
	return m
}

func (m *mockLiteLLM) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+m.masterKey {
			http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (m *mockLiteLLM) keyCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.keys)
}

func (m *mockLiteLLM) handleGenerate(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.generateErr {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
		return
	}

	var body map[string]interface{}
	_ = json.NewDecoder(r.Body).Decode(&body)
	m.lastRequest = body

	m.counter++
	value := "sk-generated-" + strconv.Itoa(m.counter)

	mk := mockKey{}
	if alias, ok := body["key_alias"].(string); ok {
		mk.Alias = alias
	}
	if models, ok := body["models"].([]interface{}); ok {
		for _, mdl := range models {
			if s, ok := mdl.(string); ok {
				mk.Models = append(mk.Models, s)
			}
		}
	}
	if budget, ok := body["max_budget"].(float64); ok {
		mk.MaxBudget = &budget
	}
	if dur, ok := body["duration"].(string); ok {
		mk.Duration = dur
	}
	m.keys[value] = mk

	writeJSON(w, map[string]interface{}{
		"key":        value,
		"key_name":   value,
		"token_id":   "tok-" + strconv.Itoa(m.counter),
		"models":     mk.Models,
		"max_budget": mk.MaxBudget,
	})
}

func (m *mockLiteLLM) handleDelete(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var body struct {
		Keys []string `json:"keys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	deleted := 0
	for _, k := range body.Keys {
		if _, ok := m.keys[k]; ok {
			delete(m.keys, k)
			deleted++
		}
	}
	writeJSON(w, map[string]interface{}{"deleted_keys": deleted})
}

func (m *mockLiteLLM) handleUpdate(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var body map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, `{"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	key, _ := body["key"].(string)
	mk, ok := m.keys[key]
	if !ok {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	if dur, ok := body["duration"].(string); ok {
		mk.Duration = dur
	}
	if budget, ok := body["max_budget"].(float64); ok {
		mk.MaxBudget = &budget
	}
	m.keys[key] = mk
	writeJSON(w, map[string]interface{}{"key": key})
}

func (m *mockLiteLLM) handleInfo(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()

	key := r.URL.Query().Get("key")
	mk, ok := m.keys[key]
	if !ok {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	writeJSON(w, map[string]interface{}{
		"key": key,
		"info": map[string]interface{}{
			"models":     mk.Models,
			"max_budget": mk.MaxBudget,
			"key_name":   mk.Alias,
		},
	})
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}

// writeTestConfig stores a config pointing at the given base URL.
func writeTestConfig(t *testing.T, b *litellmBackend, s logical.Storage, baseURL, masterKey string) {
	t.Helper()
	resp, err := b.HandleRequest(context.Background(), &logical.Request{
		Operation: logical.CreateOperation,
		Path:      "config",
		Storage:   s,
		Data: map[string]interface{}{
			"base_url":   baseURL,
			"master_key": masterKey,
		},
	})
	if err != nil || (resp != nil && resp.IsError()) {
		t.Fatalf("failed to write config: err=%v resp=%v", err, resp)
	}
}
