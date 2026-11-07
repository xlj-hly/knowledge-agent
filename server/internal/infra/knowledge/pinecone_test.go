package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

func TestPineconeDataPlaneDimensionScopeAndPartialFailure(t *testing.T) {
	var mu sync.Mutex
	dimension, upserts, deletes := 2, 0, 0
	badCount, foreign := false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Api-Key") != "secret" || r.Header.Get("X-Pinecone-API-Version") != "2025-10" {
			t.Error("missing protocol auth/version")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		switch r.URL.Path {
		case "/describe_index_stats":
			_ = json.NewEncoder(w).Encode(map[string]any{"dimension": dimension, "metric": "cosine", "vectorType": "dense"})
		case "/vectors/delete":
			if !strings.HasPrefix(fmt.Sprint(body["namespace"]), "agent_") || !strings.Contains(fmt.Sprint(body["filter"]), "new") || !strings.Contains(fmt.Sprint(body["filter"]), "tenant-a") || strings.Contains(fmt.Sprint(body["filter"]), "old") {
				t.Errorf("unsafe delete=%v", body)
			}
			deletes++
			_, _ = w.Write([]byte(`{}`))
		case "/vectors/upsert":
			rows := body["vectors"].([]any)
			row := rows[0].(map[string]any)
			metadata := row["metadata"].(map[string]any)
			if metadata["tenant_id"] != "tenant-a" || metadata["knowledge_base_id"] != "base-a" || metadata["document_id"] != "doc-a" || metadata["attempt"] != "new" || metadata["chunk_id"] != "17" || row["id"] != vectorUUID("tenant-a", "base-a", "doc-a", "new", "17") {
				t.Errorf("missing metadata=%v", body)
			}
			upserts++
			count := len(rows)
			if badCount {
				count = 0
			}
			_ = json.NewEncoder(w).Encode(map[string]int{"upsertedCount": count})
		case "/query":
			if !strings.Contains(fmt.Sprint(body["filter"]), "base-a") || body["includeMetadata"] != true {
				t.Errorf("query=%v", body)
			}
			meta := map[string]string{"tenant_id": "tenant-a", "knowledge_base_id": "base-a", "document_id": "doc-a", "attempt": "new", "chunk_id": "17"}
			if foreign {
				meta["tenant_id"] = "tenant-b"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"matches": []any{map[string]any{"id": vectorUUID("tenant-a", "base-a", "doc-a", "new", "17"), "score": .9, "metadata": meta}}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	store := domain.VectorStore{Provider: "pinecone", Config: map[string]string{"endpoint": server.URL, "namespace": "agent", "api_key": "secret"}}
	driver := PineconeEntry().Driver
	ctx := context.Background()
	points := []port.VectorPoint{{ID: "17", Vector: []float32{1, 0}}}
	if err := driver.ReplaceDocument(ctx, store, "tenant-a", "base-a", "doc-a", "new", points); err != nil {
		t.Fatal(err)
	}
	hits, err := driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 3)
	if err != nil || len(hits) != 1 || hits[0].ID != "17" || hits[0].Score != .9 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
	mu.Lock()
	foreign = true
	mu.Unlock()
	if _, err := driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 3); err == nil {
		t.Fatal("foreign result accepted")
	}
	mu.Lock()
	dimension = 3
	mu.Unlock()
	if err := driver.ReplaceDocument(ctx, store, "tenant-a", "base-a", "doc-a", "new", points); err == nil {
		t.Fatal("dimension mismatch accepted")
	}
	mu.Lock()
	if deletes != 1 || upserts != 1 {
		t.Error("dimension mismatch mutated index")
	}
	dimension = 2
	badCount = true
	mu.Unlock()
	if err := driver.ReplaceDocument(ctx, store, "tenant-a", "base-a", "doc-a", "new", points); err == nil {
		t.Fatal("partial write accepted")
	}
}
