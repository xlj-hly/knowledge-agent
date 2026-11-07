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

func TestWeaviateRESTGraphQLAndBatchErrors(t *testing.T) {
	var mu sync.Mutex
	failed, foreign := false, false
	writes, deletes := 0, 0
	class := map[string]any{"class": "Agent_2", "description": "agent-vector-dimension:2", "vectorizer": "none", "vectorIndexConfig": map[string]string{"distance": "cosine"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Error("authorization missing")
		}
		var body map[string]any
		if r.Method != http.MethodGet {
			_ = json.NewDecoder(r.Body).Decode(&body)
		}
		switch {
		case r.URL.Path == "/v1/schema/Agent_2":
			_ = json.NewEncoder(w).Encode(class)
		case r.URL.Path == "/v1/schema":
			_ = json.NewEncoder(w).Encode(map[string]any{"classes": []any{class, map[string]string{"class": "Unrelated_2"}}})
		case r.URL.Path == "/v1/batch/objects" && r.Method == http.MethodDelete:
			if !strings.Contains(fmt.Sprint(body["match"]), "new") || !strings.Contains(fmt.Sprint(body["match"]), "tenant-a") || !strings.Contains(fmt.Sprint(body["match"]), "doc-a") {
				t.Errorf("unsafe delete=%v", body)
			}
			deletes++
			_, _ = w.Write([]byte(`{"results":{"matches":1,"successful":1,"failed":0}}`))
		case r.URL.Path == "/v1/batch/objects" && r.Method == http.MethodPost:
			rows := body["objects"].([]any)
			row := rows[0].(map[string]any)
			props := row["properties"].(map[string]any)
			if row["class"] != "Agent_2" || row["id"] != vectorUUID("tenant-a", "base-a", "doc-a", "new", "17") || props["tenant_id"] != "tenant-a" || props["attempt"] != "new" {
				t.Errorf("batch=%v", body)
			}
			writes++
			status := "SUCCESS"
			if failed {
				status = "FAILED"
			}
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"result": map[string]string{"status": status}}})
		case r.URL.Path == "/v1/graphql":
			query := fmt.Sprint(body["query"])
			if !strings.Contains(query, "tenant_id") || !strings.Contains(query, "base-a") || !strings.Contains(query, "nearVector") {
				t.Errorf("query=%s", query)
			}
			tenant := "tenant-a"
			if foreign {
				tenant = "tenant-b"
			}
			row := map[string]any{"tenant_id": tenant, "knowledge_base_id": "base-a", "document_id": "doc-a", "attempt": "new", "chunk_id": "17", "_additional": map[string]any{"id": vectorUUID("tenant-a", "base-a", "doc-a", "new", "17"), "distance": .2}}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"Get": map[string]any{"Agent_2": []any{row}}}})
		default:
			t.Errorf("unexpected request=%s %s", r.Method, r.URL)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	store := domain.VectorStore{Provider: "weaviate", Config: map[string]string{"endpoint": server.URL, "collection": "Agent", "api_key": "secret"}}
	driver := WeaviateEntry().Driver
	ctx := context.Background()
	points := []port.VectorPoint{{ID: "17", Vector: []float32{1, 0}}}
	if err := driver.Test(ctx, store); err != nil {
		t.Fatal(err)
	}
	if err := driver.ReplaceDocument(ctx, store, "tenant-a", "base-a", "doc-a", "new", points); err != nil {
		t.Fatal(err)
	}
	hits, err := driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 3)
	if err != nil || len(hits) != 1 || hits[0].ID != "17" || hits[0].Score != .8 {
		t.Fatalf("hits=%v %v", hits, err)
	}
	mu.Lock()
	foreign = true
	failed = true
	mu.Unlock()
	if _, err := driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 3); err == nil {
		t.Fatal("foreign result accepted")
	}
	if err := driver.ReplaceDocument(ctx, store, "tenant-a", "base-a", "doc-a", "new", points); err == nil {
		t.Fatal("batch failure accepted")
	}
	mu.Lock()
	defer mu.Unlock()
	if writes != 2 || deletes != 2 {
		t.Fatalf("writes=%d deletes=%d", writes, deletes)
	}
}
