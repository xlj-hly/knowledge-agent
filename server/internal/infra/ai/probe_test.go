package ai

import (
	"context"
	domain "go-server/internal/domain/ai"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProbeDetectsEmbeddingDimensionAndListsModels(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Error("missing configured credentials")
		}
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"my-embed"}]}`))
		case "/v1/embeddings":
			_, _ = w.Write([]byte(`{"data":[{"embedding":[0.3,0.4,0.5]}]}`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer s.Close()
	m := domain.Model{Provider: "openai-compatible", Kind: "embedding", Endpoint: s.URL, ModelName: "my-embed", APIKey: "test-key"}
	p := NewProbe(builtinRegistry(t))
	d, err := p.Test(context.Background(), m)
	if err != nil || d != 3 {
		t.Fatalf("dimension=%d error=%v", d, err)
	}
	options, err := p.Discover(context.Background(), m)
	if err != nil || len(options) != 1 || options[0].ID != "my-embed" {
		t.Fatalf("options=%+v error=%v", options, err)
	}
}
func TestProbeRejectsUpstreamErrorsWithoutSecretEcho(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"error":"sensitive-api-key"}`))
	}))
	defer s.Close()
	_, err := NewProbe(builtinRegistry(t)).Test(context.Background(), domain.Model{Provider: "openai", Kind: "chat", Endpoint: s.URL, ModelName: "m", APIKey: "sensitive-api-key"})
	if err == nil || strings.Contains(err.Error(), "sensitive-api-key") {
		t.Fatal("upstream failure was hidden or leaked credentials")
	}
}
