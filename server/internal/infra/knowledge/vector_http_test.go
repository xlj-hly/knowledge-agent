package knowledge

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/httpclient"
)

func TestVectorHTTPBoundedErrorsAndCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/secret":
			w.WriteHeader(401)
			_, _ = w.Write([]byte("api-key=do-not-leak"))
		case "/huge":
			_, _ = w.Write([]byte(strings.Repeat("x", (8<<20)+1)))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	store := domain.VectorStore{Config: map[string]string{"endpoint": server.URL}}
	for _, path := range []string{"/secret", "/huge"} {
		_, err := vectorJSON(context.Background(), httpclient.New(), store, http.MethodGet, path, nil, nil, nil)
		if err == nil || strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), server.URL) {
			t.Fatalf("unsafe error=%v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := vectorJSON(ctx, httpclient.New(), store, http.MethodGet, "/", nil, nil, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel=%v", err)
	}
}

func TestVectorPointValidationAndDeterministicIdentity(t *testing.T) {
	for _, points := range [][]port.VectorPoint{
		{{ID: "1", Vector: []float32{0, 0}}},
		{{ID: "1", Vector: []float32{float32(math.NaN())}}},
		{{ID: "1", Vector: []float32{1}}, {ID: "1", Vector: []float32{1}}},
		{{ID: "1", Vector: []float32{1}}, {ID: "2", Vector: []float32{1, 1}}},
	} {
		if _, err := vectorPoints(points); err == nil {
			t.Fatal("invalid points accepted")
		}
	}
	first := vectorUUID("a", "bc", "doc", "old", "17")
	if first != vectorUUID("a", "bc", "doc", "old", "17") || first == vectorUUID("ab", "c", "doc", "old", "17") || first == vectorUUID("a", "bc", "doc", "new", "17") {
		t.Fatal("remote identity does not isolate scope and attempt")
	}
}

func TestVectorEndpointNormalizationPreservesCompleteInput(t *testing.T) {
	entries := append(ChromaEntries(), PineconeEntry(), WeaviateEntry(), AstraEntry())
	for _, entry := range entries {
		t.Run(entry.Definition.ID, func(t *testing.T) {
			registry, err := NewVectors(entry)
			if err != nil {
				t.Fatal(err)
			}
			config := map[string]string{"endpoint": "http://localhost:9000", "api_key": "saved-secret"}
			for _, field := range entry.Definition.Fields {
				if _, exists := config[field.Key]; !exists && field.Default != "" {
					config[field.Key] = field.Default
				}
			}
			for _, key := range []string{"tenant", "database"} {
				if entry.Definition.ID == "chromacloud" {
					config[key] = "example"
				}
			}
			old := domain.VectorStore{ID: "1", TenantID: "1", Name: "连接", Provider: entry.Definition.ID, Config: config}
			candidate := old
			candidate.Config = make(map[string]string, len(config))
			for key, value := range config {
				candidate.Config[key] = value
			}
			candidate.Config["endpoint"] = " http://localhost:9000/ "
			prepared, err := registry.PrepareStore(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Config["api_key"] != "saved-secret" || prepared.Config["endpoint"] != "http://localhost:9000" || registry.ImmutableChanged(old, prepared) {
				t.Fatalf("identity or secret lost: %v", prepared.Config)
			}
			if candidate.Config["endpoint"] != " http://localhost:9000/ " || old.Config["api_key"] != "saved-secret" {
				t.Fatal("normalization mutated input")
			}
		})
	}
}

func TestVectorProviderConfigurationErrorsPreserveInvalidInput(t *testing.T) {
	for _, tc := range []struct {
		entry  Entry
		config map[string]string
	}{
		{ChromaEntries()[0], map[string]string{"endpoint": "http://localhost", "collection": "a!", "tenant": "default_tenant", "database": "default_database"}},
		{PineconeEntry(), map[string]string{"endpoint": "http://localhost", "namespace": "!", "api_key": "secret"}},
		{WeaviateEntry(), map[string]string{"endpoint": "http://localhost", "collection": "lowercase"}},
		{AstraEntry(), map[string]string{"endpoint": "http://localhost", "namespace": "bad!", "collection": "agent", "api_key": "secret"}},
	} {
		t.Run(tc.entry.Definition.ID, func(t *testing.T) {
			err := tc.entry.Validate(domain.VectorStore{Provider: tc.entry.Definition.ID, Config: tc.config})
			var invalid *saas.Error
			if !errors.As(err, &invalid) || invalid.Code != "invalid_input" {
				t.Fatalf("configuration error must be invalid_input: %v", err)
			}
		})
	}
}

func TestVectorScopedHitRejectsInvalidCosineAndClampsRounding(t *testing.T) {
	metadata := map[string]string{"tenant_id": "tenant", "knowledge_base_id": "base", "document_id": "doc", "attempt": "attempt", "chunk_id": "71"}
	for _, score := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), 1.000002, -1.000002, 2, -2} {
		if _, err := vectorScopedHit(metadata, "tenant", "base", "71", score); err == nil {
			t.Errorf("非法cosine被接受: %v", score)
		}
	}
	for _, tc := range []struct{ score, want float64 }{{-1, -1}, {0, 0}, {0.8, 0.8}, {1, 1}, {1.0000005, 1}, {-1.0000005, -1}} {
		hit, err := vectorScopedHit(metadata, "tenant", "base", "71", tc.score)
		if err != nil || hit.ID != "71" || hit.Score != tc.want {
			t.Errorf("cosine=%v: hit=%+v err=%v want=%v", tc.score, hit, err, tc.want)
		}
	}
	if _, err := vectorScopedHit(metadata, "another-tenant", "base", "71", 0.8); err == nil {
		t.Fatal("分数归一绕过所属范围检查")
	}
}
