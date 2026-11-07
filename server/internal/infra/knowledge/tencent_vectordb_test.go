package knowledge

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

func tencentTestCollection(dim int) map[string]any {
	indexes := []any{map[string]any{"fieldName": "id", "fieldType": "string", "indexType": "primaryKey"}, map[string]any{"fieldName": "vector", "fieldType": "vector", "indexType": "HNSW", "dimension": dim, "metricType": "COSINE"}}
	for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"} {
		indexes = append(indexes, map[string]string{"fieldName": key, "fieldType": "string", "indexType": "filter"})
	}
	return map[string]any{"collection": "agent_knowledge_2", "indexes": indexes}
}

func TestTencentVectorRESTLifecycle(t *testing.T) {
	exists := false
	written := []map[string]any{}
	deletes := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer account=root&api_key=test-api-key" {
			t.Error("腾讯向量库协议授权头错误")
		}
		var input map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if string(input["database"]) != `"agent"` {
			t.Error("未传递数据库范围")
		}
		result := map[string]any{"code": 0}
		switch r.URL.Path {
		case "/collection/list":
			if exists {
				result["collections"] = []any{tencentTestCollection(2), map[string]string{"collection": "another_2"}}
			} else {
				result["collections"] = []any{}
			}
		case "/collection/create":
			exists = true
			var indexes []map[string]any
			if err := json.Unmarshal(input["indexes"], &indexes); err != nil {
				t.Error(err)
			}
			if len(indexes) != 7 || indexes[1]["metricType"] != "COSINE" || indexes[1]["dimension"] != float64(2) {
				t.Error("集合未声明必要维度或过滤索引")
			}
		case "/document/upsert":
			var documents []map[string]any
			if err := json.Unmarshal(input["documents"], &documents); err != nil {
				t.Error(err)
			}
			written = append(written, documents...)
			result["affectedCount"] = len(documents)
		case "/document/search":
			var search struct {
				Filter  string      `json:"filter"`
				Vectors [][]float32 `json:"vectors"`
				Limit   int         `json:"limit"`
			}
			if err := json.Unmarshal(input["search"], &search); err != nil {
				t.Error(err)
			}
			if search.Filter != `knowledge_base_id = "base-a" and tenant_id = "tenant-a"` || search.Limit != 3 || len(search.Vectors) != 1 {
				t.Errorf("查询过滤/数量错误: %#v", search)
			}
			if string(input["readConsistency"]) != `"strongConsistency"` {
				t.Error("未请求强一致读取")
			}
			result["documents"] = []any{[]any{map[string]any{"id": "remote-id", "chunk_id": "71", "tenant_id": "tenant-a", "knowledge_base_id": "base-a", "document_id": "doc-a", "attempt": "attempt-2", "score": 0.8}}}
		case "/document/delete":
			if string(input["collection"]) != `"agent_knowledge_2"` {
				t.Error("删除扩展到其他集合")
			}
			var query struct {
				Filter string `json:"filter"`
			}
			if err := json.Unmarshal(input["query"], &query); err != nil {
				t.Error(err)
			}
			deletes = append(deletes, query.Filter)
		default:
			t.Errorf("未预期路径: %s", r.URL.Path)
		}
		protocolReply(t, w, result)
	}))
	defer server.Close()
	s := domain.VectorStore{TenantID: "tenant-a", Provider: "tencent-vectordb", Config: map[string]string{"endpoint": server.URL, "database": "agent", "collection": "agent_knowledge", "username": "root", "api_key": "test-api-key"}}
	v := TencentVectorDBEntry().Driver
	ctx := context.Background()
	if err := v.Test(ctx, s); err != nil {
		t.Fatal(err)
	}
	for _, attempt := range []string{"attempt-1", "attempt-2"} {
		if err := v.ReplaceDocument(ctx, s, "tenant-a", "base-a", "doc-a", attempt, []port.VectorPoint{{ID: "71", Vector: []float32{1, 0}}}); err != nil {
			t.Fatal(err)
		}
	}
	if len(written) != 2 || written[0]["id"] == written[1]["id"] || written[1]["chunk_id"] != "71" || written[1]["tenant_id"] != "tenant-a" || written[1]["knowledge_base_id"] != "base-a" || written[1]["document_id"] != "doc-a" || written[1]["attempt"] != "attempt-2" {
		t.Fatalf("写入缺少处理代隔离: %#v", written)
	}
	hits, err := v.Search(ctx, s, "tenant-a", "base-a", []float32{1, 0}, 3)
	if err != nil || len(hits) != 1 || hits[0].ID != "71" || math.Abs(hits[0].Score-0.8) > 1e-6 {
		t.Fatalf("未保留业务ID或cosine: %#v %v", hits, err)
	}
	if err := v.DeleteAttempt(ctx, s, "tenant-a", "base-a", "doc-a", "attempt-1"); err != nil {
		t.Fatal(err)
	}
	if len(deletes) != 1 || deletes[0] != `attempt = "attempt-1" and document_id = "doc-a" and knowledge_base_id = "base-a" and tenant_id = "tenant-a"` {
		t.Fatalf("未限定旧代: %#v", deletes)
	}
	if err := v.DeleteDocument(ctx, s, "tenant-a", "base-a", "doc-a"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(deletes[1], "attempt") || !strings.Contains(deletes[1], `document_id = "doc-a"`) {
		t.Error("文档删除范围错误")
	}
}

func TestTencentVectorRejectsUnacknowledgedUpsert(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/collection/list" {
			protocolReply(t, w, map[string]any{"code": 0, "collections": []any{tencentTestCollection(2)}})
		} else {
			protocolReply(t, w, map[string]any{"code": 0, "affectedCount": 0})
		}
	}))
	defer server.Close()
	v := &tencentVectorDriver{client: server.Client()}
	s := domain.VectorStore{Config: map[string]string{"endpoint": server.URL, "database": "agent", "collection": "agent_knowledge", "username": "root", "api_key": "test-api-key"}}
	if err := v.ReplaceDocument(context.Background(), s, "tenant", "base", "doc", "a", []port.VectorPoint{{ID: "1", Vector: []float32{1, 0}}}); err == nil {
		t.Fatal("未确认写入条数就接受成功")
	}
}

func TestTencentVectorRejectsMalformedFailedAndForeignResults(t *testing.T) {
	for _, mode := range []string{"missing-code", "failed", "foreign", "wrong-dimension", "bad-results"} {
		t.Run(mode, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				if mode == "missing-code" {
					protocolReply(t, w, map[string]any{"collections": []any{}})
					return
				}
				if mode == "failed" {
					protocolReply(t, w, map[string]any{"code": 14003, "msg": "test-api-key unauthorized"})
					return
				}
				if r.URL.Path == "/collection/list" {
					dim := 2
					if mode == "wrong-dimension" {
						dim = 3
					}
					protocolReply(t, w, map[string]any{"code": 0, "collections": []any{tencentTestCollection(dim)}})
					return
				}
				if mode == "bad-results" {
					protocolReply(t, w, map[string]any{"code": 0, "documents": "invalid"})
					return
				}
				protocolReply(t, w, map[string]any{"code": 0, "documents": []any{[]any{map[string]any{"chunk_id": "71", "tenant_id": "other", "knowledge_base_id": "base", "document_id": "doc", "attempt": "a", "score": 0.8}}}})
			}))
			defer server.Close()
			s := domain.VectorStore{TenantID: "tenant", Config: map[string]string{"endpoint": server.URL, "database": "agent", "collection": "agent_knowledge", "username": "root", "api_key": "test-api-key"}}
			v := &tencentVectorDriver{client: server.Client()}
			if _, err := v.Search(context.Background(), s, "tenant", "base", []float32{1, 0}, 3); err == nil || strings.Contains(err.Error(), "test-api-key") {
				t.Fatalf("未拒绝或脱敏 %s: %v", mode, err)
			}
			before := requests
			if err := v.ReplaceDocument(context.Background(), s, "another", "base", "doc", "a", []port.VectorPoint{{ID: "1", Vector: []float32{1, 0}}}); err == nil || requests != before {
				t.Fatal("外企业范围触发写入")
			}
			if err := v.DeleteAttempt(context.Background(), s, "tenant", "base", "doc", ""); err == nil {
				t.Fatal("接受空处理代")
			}
		})
	}
}
