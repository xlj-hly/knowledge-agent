package knowledge

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

func protocolReply(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Error(err)
	}
}

func milvusSchema(dimension int) map[string]any {
	return map[string]any{"fields": []any{map[string]any{"name": "id", "type": "VarChar", "primaryKey": true}, map[string]any{"name": "vector", "type": "FloatVector", "params": []any{map[string]string{"key": "dim", "value": fmt.Sprint(dimension)}}}}, "indexes": []any{map[string]string{"fieldName": "vector", "metricType": "COSINE"}}, "enableDynamicField": true}
}

func TestMilvusRESTLifecycle(t *testing.T) {
	for _, brand := range []string{"milvus", "zilliz"} {
		t.Run(brand, func(t *testing.T) {
			exists := false
			written := []map[string]any{}
			deletes := []string{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer test-token" {
					t.Errorf("协议或授权头无效")
				}
				var input map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				if string(input["dbName"]) != `"default"` {
					t.Errorf("未传递数据库")
				}
				var data any = map[string]any{}
				switch r.URL.Path {
				case "/v2/vectordb/collections/list":
					if exists {
						data = []string{"agent_knowledge_2", "another_2"}
					} else {
						data = []string{}
					}
				case "/v2/vectordb/collections/has":
					data = map[string]bool{"has": exists}
				case "/v2/vectordb/collections/create":
					exists = true
					var schema struct {
						Fields []struct {
							FieldName string            `json:"fieldName"`
							DataType  string            `json:"dataType"`
							IsPrimary bool              `json:"isPrimary"`
							Params    map[string]string `json:"elementTypeParams"`
						} `json:"fields"`
					}
					if err := json.Unmarshal(input["schema"], &schema); err != nil {
						t.Error(err)
					}
					if len(schema.Fields) != 2 || schema.Fields[0].FieldName != "id" || !schema.Fields[0].IsPrimary || schema.Fields[1].Params["dim"] != "2" {
						t.Error("建集合未固定主键、维度")
					}
				case "/v2/vectordb/collections/describe":
					data = milvusSchema(2)
				case "/v2/vectordb/collections/load":
				case "/v2/vectordb/entities/upsert":
					var rows []map[string]any
					if err := json.Unmarshal(input["data"], &rows); err != nil {
						t.Error(err)
					}
					written = append(written, rows...)
					data = map[string]int{"upsertCount": len(rows)}
				case "/v2/vectordb/entities/delete":
					var filter string
					_ = json.Unmarshal(input["filter"], &filter)
					deletes = append(deletes, filter)
					if string(input["collectionName"]) != `"agent_knowledge_2"` {
						t.Error("删除扩展到其他集合")
					}
				case "/v2/vectordb/entities/search":
					if string(input["filter"]) != `"knowledge_base_id == \"base-a\" and tenant_id == \"tenant-a\""` {
						t.Errorf("检索过滤错误: %s", input["filter"])
					}
					data = []any{map[string]any{"id": "remote-id", "chunk_id": "71", "tenant_id": "tenant-a", "knowledge_base_id": "base-a", "document_id": "doc-a", "attempt": "attempt-2", "distance": 0.8}}
				default:
					t.Errorf("未预期路径: %s", r.URL.Path)
				}
				protocolReply(t, w, map[string]any{"code": 0, "data": data})
			}))
			defer server.Close()
			store := domain.VectorStore{TenantID: "tenant-a", Provider: brand, Config: map[string]string{"endpoint": server.URL, "token": "test-token", "database": "default", "collection": "agent_knowledge"}}
			var driver port.Vectors
			for _, entry := range MilvusEntries() {
				if entry.Definition.ID == brand {
					driver = entry.Driver
				}
			}
			ctx := context.Background()
			if err := driver.Test(ctx, store); err != nil {
				t.Fatal(err)
			}
			for _, attempt := range []string{"attempt-1", "attempt-2"} {
				if err := driver.ReplaceDocument(ctx, store, "tenant-a", "base-a", "doc-a", attempt, []port.VectorPoint{{ID: "71", Vector: []float32{1, 0}}}); err != nil {
					t.Fatal(err)
				}
			}
			if len(written) != 2 || written[0]["id"] == written[1]["id"] || written[1]["chunk_id"] != "71" || written[1]["tenant_id"] != "tenant-a" || written[1]["knowledge_base_id"] != "base-a" || written[1]["document_id"] != "doc-a" || written[1]["attempt"] != "attempt-2" {
				t.Fatalf("写入缺少隔离元数据: %#v", written)
			}
			hits, err := driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 3)
			if err != nil || len(hits) != 1 || hits[0].ID != "71" || math.Abs(hits[0].Score-0.8) > 1e-6 {
				t.Fatalf("检索未返回业务ID/cosine: %#v %v", hits, err)
			}
			if err := driver.DeleteAttempt(ctx, store, "tenant-a", "base-a", "doc-a", "attempt-1"); err != nil {
				t.Fatal(err)
			}
			if len(deletes) != 1 || deletes[0] != `attempt == "attempt-1" and document_id == "doc-a" and knowledge_base_id == "base-a" and tenant_id == "tenant-a"` {
				t.Fatalf("未限定旧处理代: %#v", deletes)
			}
			if err := driver.DeleteDocument(ctx, store, "tenant-a", "base-a", "doc-a"); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(deletes[1], "attempt") || !strings.Contains(deletes[1], `document_id == "doc-a"`) {
				t.Fatal("文档删除范围错误")
			}
		})
	}
}

func TestMilvusRejectsBadSchemaAndServiceErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/vectordb/collections/list":
			protocolReply(t, w, map[string]any{"code": 0, "data": []string{"agent_knowledge_2"}})
		case "/v2/vectordb/collections/describe":
			protocolReply(t, w, map[string]any{"code": 0, "data": milvusSchema(3)})
		default:
			protocolReply(t, w, map[string]any{"code": 1800, "message": "secret-token upstream failure"})
		}
	}))
	defer server.Close()
	store := domain.VectorStore{Config: map[string]string{"endpoint": server.URL, "database": "default", "collection": "agent_knowledge"}}
	v := &milvusDriver{client: server.Client()}
	if _, err := v.Search(context.Background(), store, "tenant", "base", []float32{1, 0}, 3); err == nil {
		t.Fatal("接受了错误维度schema")
	}
	if err := v.call(context.Background(), store, "entities/upsert", map[string]any{}, nil); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Fatal("未拒绝或脱敏服务端错误")
	}
	if err := v.DeleteAttempt(context.Background(), store, "tenant", "base", "doc", ""); err == nil {
		t.Fatal("接受空处理代")
	}
	if err := v.ReplaceDocument(context.Background(), store, "tenant", "base", "doc", "a", []port.VectorPoint{{ID: "1", Vector: []float32{0, 0}}}); err == nil {
		t.Fatal("接受零向量")
	}
}

func TestMilvusRejectsMissingStatusNullDataAndForeignHits(t *testing.T) {
	for _, mode := range []string{"missing-status", "null-data", "foreign"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "missing-status" {
					protocolReply(t, w, map[string]any{"data": []string{}})
					return
				}
				if mode == "null-data" {
					protocolReply(t, w, map[string]any{"code": 0, "data": nil})
					return
				}
				switch r.URL.Path {
				case "/v2/vectordb/collections/list":
					protocolReply(t, w, map[string]any{"code": 0, "data": []string{"agent_knowledge_2"}})
				case "/v2/vectordb/collections/describe":
					protocolReply(t, w, map[string]any{"code": 0, "data": milvusSchema(2)})
				default:
					protocolReply(t, w, map[string]any{"code": 0, "data": []any{map[string]any{"tenant_id": "another", "knowledge_base_id": "base", "document_id": "doc", "attempt": "a", "chunk_id": "1", "distance": 0.8}}})
				}
			}))
			defer server.Close()
			store := domain.VectorStore{Config: map[string]string{"endpoint": server.URL, "database": "default", "collection": "agent_knowledge"}}
			v := &milvusDriver{client: server.Client()}
			if mode == "foreign" {
				if _, err := v.Search(context.Background(), store, "tenant", "base", []float32{1, 0}, 3); err == nil {
					t.Fatal("接受外租户命中")
				}
			} else if err := v.Test(context.Background(), store); err == nil {
				t.Fatal("接受缺失状态或null数据")
			}
		})
	}
}

func TestHTTPVectorEntriesNormalizeCompleteConfiguration(t *testing.T) {
	entries := append(MilvusEntries(), ElasticsearchEntries()...)
	entries = append(entries, TencentVectorDBEntry())
	registry, err := NewVectors(entries...)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		t.Run(entry.Definition.ID, func(t *testing.T) {
			config := map[string]string{}
			for _, field := range entry.Definition.Fields {
				if field.Default != "" {
					config[field.Key] = field.Default
				}
			}
			config["endpoint"] = "https://example.test"
			secret := "token"
			if entry.Definition.ID == "tencent-vectordb" {
				secret = "api_key"
			} else if entry.Definition.ID == "elasticsearch" || entry.Definition.ID == "opensearch" {
				secret = "password"
				config["username"] = "alice"
			}
			config[secret] = "saved-test-secret"
			old := domain.VectorStore{ID: "7", TenantID: "tenant", Name: "测试连接", Provider: entry.Definition.ID, Config: config}
			candidate := old
			candidate.Config = cloneConfig(config)
			candidate.Config["endpoint"] = " https://example.test/ "
			prepared, err := registry.PrepareStore(candidate)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.Config[secret] != "saved-test-secret" || prepared.Config["endpoint"] != "https://example.test" || candidate.Config[secret] != "saved-test-secret" || candidate.Config["endpoint"] != " https://example.test/ " || registry.ImmutableChanged(old, prepared) {
				t.Fatal("端点等价编辑丢失完整配置或污染输入")
			}
			if config["username"] != "" {
				next := prepared
				next.Config = cloneConfig(prepared.Config)
				next.Config["username"] = "another-user"
				if !registry.ImmutableChanged(prepared, next) {
					t.Fatal("账户变化未改变连接身份")
				}
			}
		})
	}
}

func TestHTTPVectorDriversSplitWritesIntoBoundedBatches(t *testing.T) {
	for _, protocol := range []string{"milvus", "elasticsearch", "tencent"} {
		t.Run(protocol, func(t *testing.T) {
			batches := []int{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/_bulk") {
					var row json.RawMessage
					decoder := json.NewDecoder(r.Body)
					count := 0
					for decoder.More() {
						if err := decoder.Decode(&row); err != nil {
							t.Error(err)
							break
						}
						if err := decoder.Decode(&row); err != nil {
							t.Error(err)
							break
						}
						count++
					}
					batches = append(batches, count)
					items := make([]any, count)
					for i := range items {
						items[i] = map[string]any{"index": map[string]int{"status": 201}}
					}
					protocolReply(t, w, map[string]any{"errors": false, "items": items})
					return
				}
				if strings.HasSuffix(r.URL.Path, "/_mapping") {
					protocolReply(t, w, elasticTestMapping(false, 2))
					return
				}
				var input map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
					t.Error(err)
				}
				switch r.URL.Path {
				case "/v2/vectordb/collections/has":
					protocolReply(t, w, map[string]any{"code": 0, "data": map[string]bool{"has": true}})
				case "/v2/vectordb/collections/describe":
					protocolReply(t, w, map[string]any{"code": 0, "data": milvusSchema(2)})
				case "/v2/vectordb/collections/load":
					protocolReply(t, w, map[string]any{"code": 0})
				case "/collection/list":
					protocolReply(t, w, map[string]any{"code": 0, "collections": []any{tencentTestCollection(2)}})
				case "/v2/vectordb/entities/upsert", "/document/upsert":
					key := "data"
					if protocol == "tencent" {
						key = "documents"
					}
					var rows []json.RawMessage
					if err := json.Unmarshal(input[key], &rows); err != nil {
						t.Error(err)
					}
					batches = append(batches, len(rows))
					if protocol == "milvus" {
						protocolReply(t, w, map[string]any{"code": 0, "data": map[string]int{"upsertCount": len(rows)}})
					} else {
						protocolReply(t, w, map[string]any{"code": 0, "affectedCount": len(rows)})
					}
				default:
					t.Errorf("未预期路径: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			s := domain.VectorStore{Config: map[string]string{"endpoint": server.URL, "collection": "agent_knowledge", "index": "agent_knowledge", "database": "default", "username": "root", "api_key": "test-api-key"}}
			var driver port.Vectors
			switch protocol {
			case "milvus":
				driver = &milvusDriver{client: server.Client()}
			case "elasticsearch":
				delete(s.Config, "api_key")
				driver = &elasticsearchDriver{client: server.Client()}
			case "tencent":
				driver = &tencentVectorDriver{client: server.Client()}
			}
			points := make([]port.VectorPoint, 65)
			for i := range points {
				points[i] = port.VectorPoint{ID: fmt.Sprint(i + 1), Vector: []float32{1, 0}}
			}
			if err := driver.ReplaceDocument(context.Background(), s, "tenant", "base", "doc", "attempt", points); err != nil {
				t.Fatal(err)
			}
			if len(batches) != 2 || batches[0] != 64 || batches[1] != 1 {
				t.Fatalf("未按64条上限拆分: %#v", batches)
			}
		})
	}
}

func TestMilvusRejectsUnacknowledgedOrPartialUpsert(t *testing.T) {
	for _, count := range []any{nil, 0} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var data any
				switch r.URL.Path {
				case "/v2/vectordb/collections/has":
					data = map[string]bool{"has": true}
				case "/v2/vectordb/collections/describe":
					data = milvusSchema(2)
				case "/v2/vectordb/entities/upsert":
					data = map[string]any{"upsertCount": count}
				default:
					data = map[string]any{}
				}
				protocolReply(t, w, map[string]any{"code": 0, "data": data})
			}))
			defer server.Close()
			v := &milvusDriver{client: server.Client()}
			s := domain.VectorStore{Config: map[string]string{"endpoint": server.URL, "database": "default", "collection": "agent_knowledge"}}
			if err := v.ReplaceDocument(context.Background(), s, "tenant", "base", "doc", "a", []port.VectorPoint{{ID: "1", Vector: []float32{1, 0}}}); err == nil {
				t.Fatal("未确认写入条数就接受成功")
			}
		})
	}
}
