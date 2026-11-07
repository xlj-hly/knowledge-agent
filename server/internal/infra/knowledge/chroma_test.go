package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

func TestChromaV2ProtocolScopesBatchesAndRejectsWrongMetric(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		t.Run(strconv.FormatBool(cloud), func(t *testing.T) {
			var mu sync.Mutex
			writes, deletes, created := 0, 0, false
			metric, hitTenant := "cosine", "tenant-a"
			metadata := map[string]string{"tenant_id": "tenant-a", "knowledge_base_id": "base-a", "document_id": "doc-a", "attempt": "new", "chunk_id": "17"}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				if r.Header.Get("X-Chroma-Token") != "secret" {
					t.Error("Chroma token missing")
				}
				root := "/api/v2/tenants/example/databases/db/collections"
				var body map[string]any
				if r.Body != nil && r.Method == http.MethodPost {
					_ = json.NewDecoder(r.Body).Decode(&body)
				}
				collection := map[string]any{"id": "collection-id", "name": "agent_2", "dimension": 2, "configuration_json": map[string]any{"hnsw": map[string]string{"space": metric}}}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == root+"/agent_2":
					if !created {
						w.WriteHeader(404)
						return
					}
					_ = json.NewEncoder(w).Encode(collection)
				case r.Method == http.MethodGet && r.URL.Path == root:
					_ = json.NewEncoder(w).Encode([]any{collection, map[string]string{"id": "unrelated", "name": "another_2"}})
				case r.Method == http.MethodPost && r.URL.Path == root:
					config, _ := body["configuration"].(map[string]any)
					kind := "hnsw"
					if cloud {
						kind = "spann"
					}
					if config[kind] == nil || body["get_or_create"] != true {
						t.Errorf("create body=%v", body)
					}
					created = true
					_ = json.NewEncoder(w).Encode(collection)
				case strings.HasSuffix(r.URL.Path, "/delete"):
					where, _ := body["where"].(map[string]any)
					terms, _ := where["$and"].([]any)
					if len(terms) != 4 || !strings.Contains(fmt.Sprint(terms), "new") || !strings.Contains(fmt.Sprint(terms), "tenant-a") {
						t.Errorf("unsafe delete=%v", body)
					}
					deletes++
					_, _ = w.Write([]byte(`{}`))
				case strings.HasSuffix(r.URL.Path, "/upsert"):
					rows, _ := body["metadatas"].([]any)
					ids, _ := body["ids"].([]any)
					if len(rows) == 0 || len(rows) > 64 || len(rows) != len(ids) {
						t.Errorf("invalid batch=%v", body)
					}
					for _, item := range rows {
						row := item.(map[string]any)
						if row["tenant_id"] != "tenant-a" || row["knowledge_base_id"] != "base-a" || row["document_id"] != "doc-a" || row["attempt"] != "new" || row["chunk_id"] == "" {
							t.Errorf("missing scope=%v", row)
						}
					}
					writes++
					_, _ = w.Write([]byte(`{}`))
				case strings.HasSuffix(r.URL.Path, "/query"):
					if !strings.Contains(fmt.Sprint(body["where"]), "tenant-a") || body["n_results"] != float64(3) {
						t.Errorf("query=%v", body)
					}
					row := map[string]string{}
					for key, value := range metadata {
						row[key] = value
					}
					row["tenant_id"] = hitTenant
					_ = json.NewEncoder(w).Encode(map[string]any{"ids": [][]string{{vectorUUID("tenant-a", "base-a", "doc-a", "new", "17")}}, "distances": [][]float64{{.2}}, "metadatas": [][]map[string]string{{row}}})
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			store := domain.VectorStore{Provider: "chroma", Config: map[string]string{"endpoint": server.URL, "tenant": "example", "database": "db", "collection": "agent", "api_key": "secret"}}
			if cloud {
				store.Provider = "chromacloud"
			}
			driver := ChromaEntries()[0].Driver
			if cloud {
				driver = ChromaEntries()[1].Driver
			}
			points := make([]port.VectorPoint, 65)
			for i := range points {
				points[i] = port.VectorPoint{ID: strconv.Itoa(i + 1), Vector: []float32{1, 0}}
			}
			if err := driver.ReplaceDocument(context.Background(), store, "tenant-a", "base-a", "doc-a", "new", points); err != nil {
				t.Fatal(err)
			}
			mu.Lock()
			good := writes == 2 && deletes == 1
			mu.Unlock()
			if !good {
				t.Fatalf("writes=%d deletes=%d", writes, deletes)
			}
			hits, err := driver.Search(context.Background(), store, "tenant-a", "base-a", []float32{1, 0}, 3)
			if err != nil || len(hits) != 1 || hits[0].ID != "17" || hits[0].Score != .8 {
				t.Fatalf("hits=%v err=%v", hits, err)
			}
			mu.Lock()
			hitTenant = "tenant-b"
			mu.Unlock()
			if _, err := driver.Search(context.Background(), store, "tenant-a", "base-a", []float32{1, 0}, 3); err == nil {
				t.Fatal("foreign hit accepted")
			}
			mu.Lock()
			metric = "l2"
			mu.Unlock()
			if err := driver.ReplaceDocument(context.Background(), store, "tenant-a", "base-a", "doc-a", "new", points); err == nil {
				t.Fatal("non-cosine collection accepted")
			}
			mu.Lock()
			defer mu.Unlock()
			if deletes != 1 || writes != 2 {
				t.Fatal("mismatch caused mutation")
			}
		})
	}
}
