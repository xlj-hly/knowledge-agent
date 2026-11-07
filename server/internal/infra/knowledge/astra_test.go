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

func TestAstraDataAPIFiltersSimilarityAndHTTP200Errors(t *testing.T) {
	var mu sync.Mutex
	foreign, apiError := false, false
	writes, deletes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Token") != "secret" {
			t.Error("Token missing")
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if apiError {
			_, _ = w.Write([]byte(`{"errors":[{"message":"secret-do-not-leak"}]}`))
			return
		}
		switch {
		case body["findCollections"] != nil:
			if r.URL.Path != "/api/json/v1/default_keyspace" {
				t.Errorf("path=%s", r.URL.Path)
			}
			_, _ = w.Write([]byte(`{"status":{"collections":[{"name":"agent_2","options":{"vector":{"dimension":2,"metric":"cosine"}}},{"name":"unrelated_2"}]}}`))
		case body["deleteMany"] != nil:
			filter := body["deleteMany"].(map[string]any)["filter"].(map[string]any)
			if len(filter) != 4 || filter["tenant_id"] != "tenant-a" || filter["knowledge_base_id"] != "base-a" || filter["document_id"] != "doc-a" || filter["attempt"] != "new" {
				t.Errorf("unsafe delete=%v", body)
			}
			deletes++
			_, _ = w.Write([]byte(`{"status":{"deletedCount":1}}`))
		case body["insertMany"] != nil:
			rows := body["insertMany"].(map[string]any)["documents"].([]any)
			row := rows[0].(map[string]any)
			if row["tenant_id"] != "tenant-a" || row["attempt"] != "new" || row["chunk_id"] != "17" || row["_id"] != vectorUUID("tenant-a", "base-a", "doc-a", "new", "17") {
				t.Errorf("insert=%v", body)
			}
			writes++
			_, _ = w.Write([]byte(`{"status":{"insertedIds":["inserted"]}}`))
		case body["find"] != nil:
			find := body["find"].(map[string]any)
			if !strings.Contains(fmt.Sprint(find["filter"]), "base-a") {
				t.Errorf("query=%v", body)
			}
			tenant := "tenant-a"
			if foreign {
				tenant = "tenant-b"
			}
			row := map[string]any{"_id": vectorUUID("tenant-a", "base-a", "doc-a", "new", "17"), "tenant_id": tenant, "knowledge_base_id": "base-a", "document_id": "doc-a", "attempt": "new", "chunk_id": "17", "$similarity": .9}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"documents": []any{row}}})
		default:
			t.Errorf("unexpected command=%v", body)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	store := domain.VectorStore{Provider: "astra", Config: map[string]string{"endpoint": server.URL, "namespace": "default_keyspace", "collection": "agent", "api_key": "secret"}}
	driver := AstraEntry().Driver
	ctx := context.Background()
	if err := driver.ReplaceDocument(ctx, store, "tenant-a", "base-a", "doc-a", "new", []port.VectorPoint{{ID: "17", Vector: []float32{1, 0}}}); err != nil {
		t.Fatal(err)
	}
	hits, err := driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 3)
	if err != nil || len(hits) != 1 || hits[0].ID != "17" || hits[0].Score != .8 {
		t.Fatalf("hits=%v err=%v", hits, err)
	}
	mu.Lock()
	foreign = true
	mu.Unlock()
	if _, err := driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 3); err == nil {
		t.Fatal("foreign scope accepted")
	}
	mu.Lock()
	apiError = true
	mu.Unlock()
	if err := driver.Test(ctx, store); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("HTTP200 errors accepted or leaked: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if writes != 1 || deletes != 1 {
		t.Fatalf("writes=%d deletes=%d", writes, deletes)
	}
}
