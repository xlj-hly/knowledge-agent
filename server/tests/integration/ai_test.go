package saas_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	ad "go-server/internal/app/ai/dto"
)

func settingsInput(value ad.Settings) ad.SettingsInput {
	return ad.SettingsInput{EmbeddingModelID: value.EmbeddingModelID, VectorStoreID: value.VectorStoreID, SystemPrompt: value.SystemPrompt, Temperature: value.Temperature, MaxTokens: value.MaxTokens, HistoryTurns: value.HistoryTurns, RetrievalMode: value.RetrievalMode, RetrievalLimit: value.RetrievalLimit, SimilarityThreshold: value.SimilarityThreshold, ChunkStrategy: value.ChunkStrategy, ChunkSize: value.ChunkSize, ChunkOverlap: value.ChunkOverlap, ParserEndpoint: value.ParserEndpoint}
}

func TestAIConfigurationPresetsDiscoveryAndTenantDefaults(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "ai.alpha.test")
	s.createTenant(t, "ai.beta.test")
	a := s.login(t, "ai.alpha.test", "tenant.admin", adminPassword)
	b := s.login(t, "ai.beta.test", "tenant.admin", adminPassword)
	var upstreamCalls atomic.Int32
	var receivedAuthorization atomic.Value
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		receivedAuthorization.Store(r.Header.Get("Authorization"))
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"fixture-embedding"}]}`))
		case "/v1/embeddings":
			_, _ = w.Write([]byte(`{"data":[{"embedding":[0.4,0.6]}]}`))
		default:
			t.Errorf("unexpected probe %s", r.URL.Path)
		}
	}))
	defer upstream.Close()
	providers := decodeSuccess[[]ad.Provider](t, s.request("GET", "ai.alpha.test", "/api/ai/providers", a.Token, nil))
	if len(providers) < 3 {
		t.Fatal("provider presets missing")
	}
	foundGroq := false
	for _, provider := range providers {
		if provider.Name == "" || provider.Description == "" || !strings.HasPrefix(provider.Logo, "data:image/") || len(provider.Kinds) == 0 {
			t.Fatalf("incomplete provider directory entry: %s", provider.ID)
		}
		if provider.ID == "groq" {
			foundGroq = provider.Endpoint == "https://api.groq.com/openai/v1" && provider.Protocol == "openai"
		}
	}
	if !foundGroq {
		t.Fatal("compatible provider module is absent from the API directory")
	}
	assertFailure(t, s.request("GET", "ai.alpha.test", "/api/ai/providers", "", nil), 401)
	for _, invalid := range []map[string]any{
		{"name": "未知服务", "kind": "chat", "provider": "unregistered", "endpoint": upstream.URL, "model_name": "fixture", "dimension": 0},
		{"name": "错误用途", "kind": "embedding", "provider": "groq", "endpoint": upstream.URL, "model_name": "fixture", "dimension": 2},
	} {
		assertFailure(t, s.request("POST", "ai.alpha.test", "/api/ai/models", a.Token, invalid), 400)
		assertFailure(t, s.request("POST", "ai.alpha.test", "/api/ai/models/test", a.Token, invalid), 400)
	}
	if upstreamCalls.Load() != 0 {
		t.Fatal("invalid provider configuration reached the upstream")
	}
	if rows := decodeSuccess[[]ad.Model](t, s.request("GET", "ai.alpha.test", "/api/ai/models", a.Token, nil)); len(rows) != 0 {
		t.Fatal("invalid provider configuration was persisted")
	}
	for _, oldID := range []string{"1", "abc", "9223372036854775808"} {
		assertFailure(t, s.request("POST", "ai.alpha.test", "/api/ai/models/discover", a.Token, map[string]any{"id": oldID, "kind": "embedding", "provider": "openai", "endpoint": upstream.URL, "dimension": 0}), 400)
	}
	presets := decodeSuccess[ad.Presets](t, s.request("GET", "ai.alpha.test", "/api/ai/presets", a.Token, nil))
	if len(presets.Prompts) < 2 || len(presets.Chunking) < 2 || len(presets.ChunkStrategies) != 3 {
		t.Fatal("prompt/chunking presets missing")
	}
	for index, strategy := range []string{"auto", "recursive", "heading"} {
		row := presets.ChunkStrategies[index]
		if row.ID != strategy || row.Name == "" || row.Description == "" {
			t.Fatalf("incomplete chunk strategy directory: %+v", row)
		}
	}
	assertFailure(t, s.request("GET", "ai.alpha.test", "/api/ai/presets", "", nil), 401)
	in := map[string]any{"name": "检测模型", "kind": "embedding", "provider": "openai-compatible", "endpoint": upstream.URL + "/v1", "model_name": "fixture-embedding", "dimension": 0, "api_key": "ai-private-test-key"}
	probe := decodeSuccess[ad.TestResult](t, s.request("POST", "ai.alpha.test", "/api/ai/models/test", a.Token, in))
	if !probe.OK || probe.Dimension != 2 || receivedAuthorization.Load() != "Bearer ai-private-test-key" {
		t.Fatalf("dimension probe %+v", probe)
	}
	in["dimension"] = probe.Dimension
	created := s.request("POST", "ai.alpha.test", "/api/ai/models", a.Token, in)
	model := decodeSuccess[ad.Model](t, created)
	if model.APIKey != "ai-private-test-key" {
		t.Fatal("configuration response did not return the submitted model key")
	}
	rows := decodeSuccess[[]ad.Model](t, s.request("GET", "ai.alpha.test", "/api/ai/models", a.Token, nil))
	if len(rows) != 1 || rows[0].APIKey != model.APIKey {
		t.Fatal("configuration read did not return the saved model key")
	}
	options := decodeSuccess[ad.Discovery](t, s.request("POST", "ai.alpha.test", "/api/ai/models/discover", a.Token, in))
	if len(options.Models) != 1 || options.Models[0].ID != "fixture-embedding" || receivedAuthorization.Load() != "Bearer ai-private-test-key" {
		t.Fatal("model discovery mismatch")
	}
	delete(in, "api_key")
	decodeSuccess[ad.TestResult](t, s.request("POST", "ai.alpha.test", "/api/ai/models/test", a.Token, in))
	if receivedAuthorization.Load() != "" {
		t.Fatal("probe inherited stored credentials instead of using the submitted configuration")
	}
	for _, supplied := range []struct {
		name, key string
		omit      bool
	}{{name: "rotate", key: "rotated-ai-key"}, {name: "clear explicitly"}, {name: "restore", key: "another-ai-key"}, {name: "clear by omission", omit: true}} {
		if supplied.omit {
			delete(in, "api_key")
		} else {
			in["api_key"] = supplied.key
		}
		saved := decodeSuccess[ad.Model](t, s.request("PUT", "ai.alpha.test", "/api/ai/models/"+model.ID, a.Token, in))
		listed := decodeSuccess[[]ad.Model](t, s.request("GET", "ai.alpha.test", "/api/ai/models", a.Token, nil))
		if saved.APIKey != supplied.key || len(listed) != 1 || listed[0].APIKey != supplied.key {
			t.Fatalf("model credential replacement failed: %s", supplied.name)
		}
	}
	assertFailure(t, s.request("GET", "ai.alpha.test", "/api/ai/models", "", nil), 401)
	if other := decodeSuccess[[]ad.Model](t, s.request("GET", "ai.beta.test", "/api/ai/models", b.Token, nil)); len(other) != 0 {
		t.Fatal("model configuration crossed tenant scopes")
	}
	assertFailure(t, s.request("PUT", "ai.beta.test", "/api/ai/models/"+model.ID, b.Token, in), 404)
	assertFailure(t, s.request("DELETE", "ai.beta.test", "/api/ai/models/"+model.ID, b.Token, nil), 404)
	settings := decodeSuccess[ad.Settings](t, s.request("GET", "ai.alpha.test", "/api/ai/settings", a.Token, nil))
	settings.EmbeddingModelID = model.ID
	settings.SystemPrompt = "企业甲提示词"
	decodeSuccess[ad.Settings](t, s.request("PUT", "ai.alpha.test", "/api/ai/settings", a.Token, settingsInput(settings)))
	other := decodeSuccess[ad.Settings](t, s.request("GET", "ai.beta.test", "/api/ai/settings", b.Token, nil))
	if other.EmbeddingModelID != "" || other.SystemPrompt == settings.SystemPrompt {
		t.Fatal("tenant defaults leaked")
	}
	assertFailure(t, s.request("DELETE", "ai.alpha.test", "/api/ai/models/"+model.ID, a.Token, nil), 409)
	in["kind"] = "chat"
	in["dimension"] = 0
	assertFailure(t, s.request("PUT", "ai.alpha.test", "/api/ai/models/"+model.ID, a.Token, in), 409)
	settings.EmbeddingModelID = ""
	decodeSuccess[ad.Settings](t, s.request("PUT", "ai.alpha.test", "/api/ai/settings", a.Token, settingsInput(settings)))
	decodeSuccess[ad.OK](t, s.request("DELETE", "ai.alpha.test", "/api/ai/models/"+model.ID, a.Token, nil))
	if rows := decodeSuccess[[]ad.Model](t, s.request("GET", "ai.alpha.test", "/api/ai/models", a.Token, nil)); len(rows) != 0 {
		t.Fatal("delete not persisted")
	}
}
