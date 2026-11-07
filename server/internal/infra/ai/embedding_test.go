package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	domain "go-server/internal/domain/ai"
)

func TestEmbeddingOrdersOpenAIResultsAndUsesConfiguredProtocol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" || r.Header.Get("Authorization") != "Bearer local-test-key" {
			t.Errorf("unexpected protocol: path=%s", r.URL.Path)
		}
		var input struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if input.Model != "configured-model" || !reflect.DeepEqual(input.Input, []string{"第一段", "第二段"}) {
			t.Errorf("unexpected request: %+v", input)
		}
		_, _ = w.Write([]byte(`{"data":[{"index":1,"embedding":[0.2,0.4]},{"index":0,"embedding":[0.8,0.6]}]}`))
	}))
	defer server.Close()
	model := domain.Model{Kind: "embedding", Provider: "openai", Endpoint: server.URL, ModelName: "configured-model", Dimension: 2, APIKey: "local-test-key"}
	vectors, err := NewEmbedder(builtinRegistry(t)).Embed(context.Background(), model, []string{"第一段", "第二段"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(vectors, [][]float32{{0.8, 0.6}, {0.2, 0.4}}) {
		t.Fatalf("wrong input order: %v", vectors)
	}
}

func TestEmbeddingRejectsInvalidUpstreamResults(t *testing.T) {
	cases := []struct{ name, body string }{
		{"missing vector", `{"data":[{"index":0,"embedding":[1,0]}]}`},
		{"duplicate index", `{"data":[{"index":0,"embedding":[1,0]},{"index":0,"embedding":[1,0]}]}`},
		{"out of range", `{"data":[{"index":0,"embedding":[1,0]},{"index":2,"embedding":[1,0]}]}`},
		{"wrong dimension", `{"data":[{"index":0,"embedding":[1]},{"index":1,"embedding":[1]}]}`},
		{"zero vector", `{"data":[{"index":0,"embedding":[0,0]},{"index":1,"embedding":[1,0]}]}`},
		{"model error", `{"error":{"message":"local-test-key must not escape"}}`},
		{"invalid numeric value", `{"data":[{"embedding":[1e999,0]},{"embedding":[1,0]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(tc.body)) }))
			defer server.Close()
			model := domain.Model{Kind: "embedding", Provider: "openai", Endpoint: server.URL + "/v1", ModelName: "m", Dimension: 2, APIKey: "local-test-key"}
			if _, err := NewEmbedder(builtinRegistry(t)).Embed(context.Background(), model, []string{"a", "b"}); err == nil {
				t.Fatal("invalid upstream response accepted")
			} else if strings.Contains(err.Error(), "local-test-key") {
				t.Fatal("upstream credentials escaped")
			}
		})
	}
}

func TestEmbeddingOllamaAndFullConfiguredEndpoint(t *testing.T) {
	for _, provider := range []string{"ollama", "openai"} {
		t.Run(provider, func(t *testing.T) {
			path := "/api/embed"
			response := `{"embeddings":[[1,0]]}`
			if provider == "openai" {
				path = "/custom/embeddings"
				response = `{"data":[{"embedding":[1,0]}]}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != path {
					t.Errorf("configured endpoint changed: %s", r.URL.Path)
				}
				var input map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				if provider == "ollama" && string(input["truncate"]) != "false" {
					t.Error("Ollama must not silently truncate documents")
				}
				_, _ = w.Write([]byte(response))
			}))
			defer server.Close()
			model := domain.Model{Kind: "embedding", Provider: provider, Endpoint: server.URL + path, ModelName: "m", Dimension: 2}
			if _, err := NewEmbedder(builtinRegistry(t)).Embed(context.Background(), model, []string{"a"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestConfiguredServiceRejectsRedirectWithoutSendingCredential(t *testing.T) {
	called := false
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	model := domain.Model{Kind: "embedding", Provider: "openai", Endpoint: source.URL, ModelName: "m", Dimension: 2, APIKey: "local-test-key"}
	_, err := NewEmbedder(builtinRegistry(t)).Embed(context.Background(), model, []string{"a"})
	if err == nil || called {
		t.Fatalf("redirect followed or accepted: called=%v err=%v", called, err)
	}
}
