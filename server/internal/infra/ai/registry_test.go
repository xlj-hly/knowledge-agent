package ai

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	chatport "go-server/internal/app/chat/port"
	domain "go-server/internal/domain/ai"
	"go-server/internal/infra/ai/providers"
)

func builtinRegistry(t *testing.T) *Registry {
	t.Helper()
	r, err := NewBuiltinRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func customDefinition() providers.Definition {
	return providers.Definition{ID: "test-cloud", Name: "测试云", Description: "自有兼容服务", Logo: "data:image/png;base64,aWNvbg==", Protocol: "openai", Endpoint: "http://localhost:9000/v1", Kinds: []string{"chat", "embedding"}, Models: []providers.Model{{ID: "embedding-test", Kind: "embedding", Dimension: 2}}, Compatibility: providers.Compatibility{RootPath: "/v1", DiscoveryPath: "/models", TokenField: "max_tokens"}}
}

func TestRegistryRejectsContradictoryRegistration(t *testing.T) {
	base := customDefinition()
	for _, tc := range []struct {
		name   string
		change func(*providers.Definition)
	}{
		{"missing identity", func(d *providers.Definition) { d.ID = "" }},
		{"unknown protocol", func(d *providers.Definition) { d.Protocol = "unregistered" }},
		{"no capability", func(d *providers.Definition) { d.Kinds = nil }},
		{"invalid capability", func(d *providers.Definition) { d.Kinds = []string{"rerank"} }},
		{"invalid candidate", func(d *providers.Definition) { d.Models[0].Kind = "rerank" }},
		{"credentials in URL", func(d *providers.Definition) { d.Endpoint = "https://secret@example.com/v1" }},
		{"query in path", func(d *providers.Definition) { d.Compatibility.DiscoveryPath = "/models?key=secret" }},
		{"invalid token parameter", func(d *providers.Definition) { d.Compatibility.TokenField = "unexpected" }},
		{"remote logo", func(d *providers.Definition) { d.Logo = "https://example.com/logo.svg" }},
		{"unsupported logo type", func(d *providers.Definition) { d.Logo = "data:text/html;base64,aWNvbg==" }},
		{"invalid base64 logo", func(d *providers.Definition) { d.Logo = "data:image/png;base64,invalid!" }},
		{"empty logo content", func(d *providers.Definition) { d.Logo = "data:image/png;base64," }},
		{"oversized logo", func(d *providers.Definition) { d.Logo = "data:image/png;base64," + strings.Repeat("a", 256*1024) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := cloneDefinition(base)
			tc.change(&d)
			if _, err := NewRegistry([]providers.Definition{d}, NewOpenAIDriver()); err == nil {
				t.Fatal("invalid registration accepted")
			}
		})
	}
	if _, err := NewRegistry([]providers.Definition{base, base}, NewOpenAIDriver()); err == nil {
		t.Fatal("duplicate provider accepted")
	}
	if _, err := NewRegistry([]providers.Definition{base}); err == nil {
		t.Fatal("provider without driver accepted")
	}
	if _, err := NewRegistry([]providers.Definition{base}, NewOpenAIDriver(), NewOpenAIDriver()); err == nil {
		t.Fatal("duplicate driver accepted")
	}
}

type chatOnlyDriver struct{ Driver }

func (chatOnlyDriver) Kinds() []string { return []string{"chat"} }

func TestRegistryRejectsCapabilityAbsentFromDriver(t *testing.T) {
	if _, err := NewRegistry([]providers.Definition{customDefinition()}, chatOnlyDriver{NewOpenAIDriver()}); err == nil {
		t.Fatal("embedding registered against chat-only driver")
	}
}

func TestBuiltinDirectoryContainsExecutableCapabilitiesAndLocalLogos(t *testing.T) {
	r := builtinRegistry(t)
	rows := r.Providers()
	chat, embedding := 0, 0
	for _, provider := range rows {
		if contains(provider.Kinds, "chat") {
			chat++
		}
		if contains(provider.Kinds, "embedding") {
			embedding++
		}
		prefix, payload, ok := strings.Cut(provider.Logo, ";base64,")
		if !ok || (prefix != "data:image/png" && prefix != "data:image/jpeg" && prefix != "data:image/svg+xml") || len(provider.Logo) > 256*1024 {
			t.Fatalf("invalid local logo for %s", provider.ID)
		}
		if data, err := base64.StdEncoding.DecodeString(payload); err != nil || len(data) == 0 {
			t.Fatalf("invalid logo bytes for %s", provider.ID)
		}
	}
	if chat != 29 || embedding != 11 {
		t.Fatalf("chat=%d embedding=%d", chat, embedding)
	}
	if _, err := NewProbe(r).Discover(context.Background(), domain.Model{Provider: "cohere", Kind: "embedding", Endpoint: "http://localhost:1"}); err == nil {
		t.Fatal("provider without discovery attempted a generic endpoint")
	}
}

func TestRegistryOwnsImmutableDefinitions(t *testing.T) {
	definition := customDefinition()
	r, err := NewRegistry([]providers.Definition{definition}, NewOpenAIDriver())
	if err != nil {
		t.Fatal(err)
	}
	definition.Kinds[0] = "unknown"
	definition.Models[0].ID = "changed-input"
	rows := r.Providers()
	rows[0].Kinds[0] = "unknown"
	rows[0].Models[0].ID = "changed-output"
	var wg sync.WaitGroup
	for range 16 {
		wg.Go(func() {
			for range 20 {
				rows := r.Providers()
				if rows[0].Kinds[0] != "chat" || rows[0].Models[0].ID != "embedding-test" {
					t.Error("catalog changed through external slice")
				}
				rows[0].Kinds[0] = "local-change"
			}
		})
	}
	wg.Wait()
	if err := r.ValidateModel(domain.Model{Name: "custom", Provider: "test-cloud", Kind: "chat", Endpoint: "http://localhost:9000", ModelName: "m"}); err != nil {
		t.Fatal(err)
	}
	if err := r.ValidateModel(domain.Model{Name: "unknown", Provider: "unknown", Kind: "chat", Endpoint: "http://localhost:9000", ModelName: "m"}); err == nil {
		t.Fatal("unregistered provider accepted")
	}
}

func TestRegisteredCompatibleProviderUsesAllModelOperations(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("configured credential missing")
		}
		switch r.URL.Path {
		case "/gateway/models":
			fmt.Fprint(w, `{"data":[{"id":"embedding-test"}]}`)
		case "/gateway/embeddings":
			fmt.Fprint(w, `{"data":[{"index":0,"embedding":[0.3,0.4]}]}`)
		case "/gateway/chat/completions":
			var body struct {
				Stream bool
				Model  string
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Model != "chat-test" {
				t.Error("wrong model")
			}
			if body.Stream {
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"已连接\"}}]}\n\ndata: [DONE]\n\n")
			} else {
				fmt.Fprint(w, `{"choices":[{"message":{"content":"已连接"}}]}`)
			}
		default:
			t.Errorf("wrong path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	d := customDefinition()
	r, err := NewRegistry([]providers.Definition{d}, NewOpenAIDriver())
	if err != nil {
		t.Fatal(err)
	}
	m := domain.Model{Name: "测试模型", Provider: d.ID, Kind: "embedding", Endpoint: server.URL + "/gateway", ModelName: "embedding-test", Dimension: 2, APIKey: "test-secret"}
	models, err := NewProbe(r).Discover(context.Background(), m)
	if err != nil || len(models) != 1 || models[0].Dimension != 2 {
		t.Fatalf("discovery=%v error=%v", models, err)
	}
	dimension, err := NewProbe(r).Test(context.Background(), m)
	if err != nil || dimension != 2 {
		t.Fatalf("dimension=%d error=%v", dimension, err)
	}
	vectors, err := NewEmbedder(r).Embed(context.Background(), m, []string{"一段资料"})
	if err != nil || len(vectors) != 1 || vectors[0][1] != 0.4 {
		t.Fatalf("vectors=%v error=%v", vectors, err)
	}
	m.Kind = "chat"
	m.ModelName = "chat-test"
	m.Dimension = 0
	if _, err := NewProbe(r).Test(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	var answer strings.Builder
	if err := NewChat(r).Stream(context.Background(), m, chatport.ChatRequest{Messages: []chatport.ChatTurn{{Role: "user", Content: "你好"}}}, func(part string) error { answer.WriteString(part); return nil }); err != nil {
		t.Fatal(err)
	}
	if answer.String() != "已连接" || len(calls) != 5 {
		t.Fatalf("answer=%q calls=%v", answer.String(), calls)
	}
}

func TestProviderDifferencesAreUsedByProbeAndChat(t *testing.T) {
	for _, provider := range []string{"moonshot", "gemini", "perplexity", "openrouter"} {
		t.Run(provider, func(t *testing.T) {
			r := builtinRegistry(t)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if provider == "perplexity" && req.URL.Path != "/chat/completions" {
					t.Errorf("unexpected root path %s", req.URL.Path)
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if (provider == "moonshot" || provider == "gemini") && body["temperature"] != nil {
					t.Error("unsupported temperature sent")
				}
				if provider == "moonshot" || provider == "openrouter" {
					if body["max_completion_tokens"] == nil || body["max_tokens"] != nil {
						t.Error("wrong output token field")
					}
				}
				if string(body["stream"]) == "true" {
					fmt.Fprint(w, "data: [DONE]\n\n")
				} else {
					fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
				}
			}))
			defer server.Close()
			m := domain.Model{Provider: provider, Kind: "chat", Endpoint: server.URL, ModelName: "configured"}
			if _, err := NewProbe(r).Test(context.Background(), m); err != nil {
				t.Fatal(err)
			}
			if err := NewChat(r).Stream(context.Background(), m, chatport.ChatRequest{Temperature: 0.7, MaxTokens: 100}, func(string) error { return nil }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOpenRouterEmbeddingDiscoveryUsesDeclaredPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/embeddings/models" {
			t.Errorf("path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"data":[{"id":"vendor/embed"}]}`)
	}))
	defer server.Close()
	models, err := NewProbe(builtinRegistry(t)).Discover(context.Background(), domain.Model{Provider: "openrouter", Kind: "embedding", Endpoint: server.URL + "/api/v1"})
	if err != nil || len(models) != 1 || models[0].ID != "vendor/embed" {
		t.Fatalf("models=%v error=%v", models, err)
	}
}
