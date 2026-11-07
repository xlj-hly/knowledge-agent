package knowledge

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

func elasticTestMapping(openSearch bool, dimension int) map[string]any {
	properties := map[string]any{}
	for _, key := range []string{"tenant_id", "knowledge_base_id", "document_id", "attempt", "chunk_id"} {
		properties[key] = map[string]string{"type": "keyword"}
	}
	if openSearch {
		properties["vector"] = map[string]any{"type": "knn_vector", "dimension": dimension, "method": map[string]string{"space_type": "cosinesimil"}}
	} else {
		properties["vector"] = map[string]any{"type": "dense_vector", "dims": dimension, "similarity": "cosine"}
	}
	return map[string]any{"agent_knowledge_2": map[string]any{"mappings": map[string]any{"properties": properties}}}
}

func TestElasticsearchRESTLifecycle(t *testing.T) {
	for _, openSearch := range []bool{false, true} {
		brand := "elasticsearch"
		if openSearch {
			brand = "opensearch"
		}
		t.Run(brand, func(t *testing.T) {
			exists := false
			written := []map[string]any{}
			ids := []string{}
			deletes := []map[string]any{}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Basic YWxpY2U6cGFzc3dvcmQ=" {
					t.Error("未传递Basic凭据")
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/":
					distribution := ""
					if openSearch {
						distribution = "opensearch"
					}
					protocolReply(t, w, map[string]any{"version": map[string]string{"number": "9.0.0", "distribution": distribution}})
				case r.Method == http.MethodGet && r.URL.Path == "/agent_knowledge_2/_mapping":
					if !exists {
						w.WriteHeader(http.StatusNotFound)
						protocolReply(t, w, map[string]string{"error": "missing"})
						return
					}
					protocolReply(t, w, elasticTestMapping(openSearch, 2))
				case r.Method == http.MethodPut && r.URL.Path == "/agent_knowledge_2":
					exists = true
					protocolReply(t, w, map[string]bool{"acknowledged": true})
				case r.URL.Path == "/agent_knowledge_2/_bulk":
					if r.Header.Get("Content-Type") != "application/x-ndjson" || r.URL.Query().Get("refresh") != "wait_for" {
						t.Error("批量写入未使用NDJSON/等待可见")
					}
					scanner := bufio.NewScanner(r.Body)
					items := []any{}
					for scanner.Scan() {
						var action map[string]struct {
							ID string `json:"_id"`
						}
						if err := json.Unmarshal(scanner.Bytes(), &action); err != nil {
							t.Error(err)
						}
						ids = append(ids, action["index"].ID)
						if !scanner.Scan() {
							t.Error("缺少批量记录")
							break
						}
						var row map[string]any
						if err := json.Unmarshal(scanner.Bytes(), &row); err != nil {
							t.Error(err)
						}
						written = append(written, row)
						items = append(items, map[string]any{"index": map[string]int{"status": 201}})
					}
					if err := scanner.Err(); err != nil {
						t.Error(err)
					}
					protocolReply(t, w, map[string]any{"errors": false, "items": items})
				case r.URL.Path == "/agent_knowledge_2/_search":
					var body struct {
						Query struct {
							ScriptScore struct {
								Query struct {
									Bool struct {
										Filter []map[string]map[string]string `json:"filter"`
									} `json:"bool"`
								} `json:"query"`
								Script struct {
									Source string `json:"source"`
									Lang   string `json:"lang"`
								} `json:"script"`
							} `json:"script_score"`
						} `json:"query"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					terms := map[string]string{}
					for _, filter := range body.Query.ScriptScore.Query.Bool.Filter {
						for key, value := range filter["term"] {
							terms[key] = value
						}
					}
					if terms["tenant_id"] != "tenant-a" || terms["knowledge_base_id"] != "base-a" || len(terms) != 2 {
						t.Errorf("检索未隔离企业/知识库: %#v", terms)
					}
					want := "cosineSimilarity(params.query, 'vector') + 1.0"
					if openSearch {
						want = "cosineSimilarity(params.query, doc['vector']) + 1.0"
					}
					if body.Query.ScriptScore.Script.Source != want || body.Query.ScriptScore.Script.Lang != "painless" {
						t.Error("未固定cosine表达式，归一可能随版本变化")
					}
					protocolReply(t, w, map[string]any{"hits": map[string]any{"hits": []any{map[string]any{"_id": "remote-id", "_score": 1.8, "_source": map[string]string{"tenant_id": "tenant-a", "knowledge_base_id": "base-a", "document_id": "doc-a", "attempt": "attempt-2", "chunk_id": "71"}}}}})
				case r.URL.Path == "/_cat/indices/agent_knowledge_*":
					protocolReply(t, w, []any{map[string]string{"index": "agent_knowledge_2"}, map[string]string{"index": "agent_knowledge_backup"}, map[string]string{"index": "agent_knowledge_archive_2"}, map[string]string{"index": "unrelated_2"}})
				case r.URL.Path == "/agent_knowledge_2/_delete_by_query":
					var body map[string]any
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Error(err)
					}
					deletes = append(deletes, body)
					protocolReply(t, w, map[string]any{"deleted": 1, "failures": []any{}, "timed_out": false})
				default:
					t.Errorf("未预期路径: %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			store := domain.VectorStore{TenantID: "tenant-a", Provider: brand, Config: map[string]string{"endpoint": server.URL, "index": "agent_knowledge", "username": "alice", "password": "password"}}
			v := &elasticsearchDriver{client: server.Client(), openSearch: openSearch}
			ctx := context.Background()
			if err := v.Test(ctx, store); err != nil {
				t.Fatal(err)
			}
			for _, attempt := range []string{"attempt-1", "attempt-2"} {
				if err := v.ReplaceDocument(ctx, store, "tenant-a", "base-a", "doc-a", attempt, []port.VectorPoint{{ID: "71", Vector: []float32{1, 0}}}); err != nil {
					t.Fatal(err)
				}
			}
			if len(written) != 2 || ids[0] == ids[1] || written[1]["tenant_id"] != "tenant-a" || written[1]["knowledge_base_id"] != "base-a" || written[1]["document_id"] != "doc-a" || written[1]["attempt"] != "attempt-2" || written[1]["chunk_id"] != "71" {
				t.Fatalf("未隔离处理代写入: %#v", written)
			}
			hits, err := v.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 3)
			if err != nil || len(hits) != 1 || hits[0].ID != "71" || math.Abs(hits[0].Score-0.8) > 1e-6 {
				t.Fatalf("cosine或业务ID错误: %#v %v", hits, err)
			}
			if err := v.DeleteAttempt(ctx, store, "tenant-a", "base-a", "doc-a", "attempt-1"); err != nil {
				t.Fatal(err)
			}
			encoded, _ := json.Marshal(deletes)
			for _, scope := range []string{`"tenant_id":"tenant-a"`, `"knowledge_base_id":"base-a"`, `"document_id":"doc-a"`, `"attempt":"attempt-1"`} {
				if !strings.Contains(string(encoded), scope) {
					t.Fatalf("删除未限定所属范围: %s", encoded)
				}
			}
		})
	}
}

func TestElasticsearchRejectsPartialBulkAndForeignHits(t *testing.T) {
	mode := "bulk"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch {
		case strings.HasSuffix(r.URL.Path, "/_mapping"):
			protocolReply(t, w, elasticTestMapping(false, 2))
		case strings.HasSuffix(r.URL.Path, "/_bulk"):
			protocolReply(t, w, map[string]any{"errors": true, "items": []any{map[string]any{"index": map[string]any{"status": 400, "error": "test-secret-key"}}}})
		case strings.HasSuffix(r.URL.Path, "/_search"):
			protocolReply(t, w, map[string]any{"hits": map[string]any{"hits": []any{map[string]any{"_score": 1.4, "_source": map[string]string{"tenant_id": "other", "knowledge_base_id": "base", "document_id": "doc", "attempt": "a", "chunk_id": "71"}}}}})
		default:
			t.Errorf("unexpected mode/path %s %s", mode, r.URL.Path)
		}
	}))
	defer server.Close()
	s := domain.VectorStore{Config: map[string]string{"endpoint": server.URL, "index": "agent_knowledge"}}
	v := &elasticsearchDriver{client: server.Client()}
	ctx := context.Background()
	if err := v.ReplaceDocument(ctx, s, "tenant", "base", "doc", "a", []port.VectorPoint{{ID: "71", Vector: []float32{1, 0}}}); err == nil || strings.Contains(err.Error(), "test-secret-key") {
		t.Fatal("未拒绝或脱敏部分写入失败")
	}
	mode = "search"
	if _, err := v.Search(ctx, s, "tenant", "base", []float32{1, 0}, 3); err == nil {
		t.Fatal("接受外企业结果")
	}
	before := requests
	s.Config["index"] = "../*"
	if err := v.ReplaceDocument(ctx, s, "tenant", "base", "doc", "a", []port.VectorPoint{{ID: "71", Vector: []float32{1, 0}}}); err == nil || requests != before {
		t.Fatal("无效索引名称发起了外部请求")
	}
}

func TestElasticsearchRawCancellationAndResponseBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, io.LimitReader(strings.NewReader(strings.Repeat("x", (8<<20)+1)), (8<<20)+1))
	}))
	defer server.Close()
	s := domain.VectorStore{Config: map[string]string{"endpoint": server.URL, "index": "agent_knowledge"}}
	v := &elasticsearchDriver{client: server.Client()}
	var output any
	if err := v.raw(context.Background(), s, "/_bulk", nil, &output); err == nil || !strings.Contains(err.Error(), "大小限制") {
		t.Fatalf("未限制bulk响应: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := v.raw(ctx, s, "/_bulk", nil, &output); err != context.Canceled {
		t.Fatalf("取消未传递: %v", err)
	}
}

func TestElasticsearchRejectsMalformedSearchAndIncompleteDelete(t *testing.T) {
	for _, mode := range []string{"empty-search", "null-hits", "partial-shards", "failed-delete"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/_mapping"):
					protocolReply(t, w, elasticTestMapping(false, 2))
				case strings.HasPrefix(r.URL.Path, "/_cat/indices/"):
					protocolReply(t, w, []any{map[string]string{"index": "agent_knowledge_2"}})
				case strings.HasSuffix(r.URL.Path, "/_delete_by_query"):
					protocolReply(t, w, map[string]any{"timed_out": false, "failures": []any{map[string]string{"reason": "test-api-key"}}})
				default:
					if mode == "null-hits" {
						protocolReply(t, w, map[string]any{"hits": nil})
					} else if mode == "partial-shards" {
						protocolReply(t, w, map[string]any{"_shards": map[string]int{"failed": 1}, "hits": map[string]any{"hits": []any{}}})
					} else {
						protocolReply(t, w, map[string]any{})
					}
				}
			}))
			defer server.Close()
			s := domain.VectorStore{Config: map[string]string{"endpoint": server.URL, "index": "agent_knowledge"}}
			v := &elasticsearchDriver{client: server.Client()}
			var err error
			if mode == "failed-delete" {
				err = v.DeleteAttempt(context.Background(), s, "tenant", "base", "doc", "a")
			} else {
				_, err = v.Search(context.Background(), s, "tenant", "base", []float32{1, 0}, 3)
			}
			if err == nil || strings.Contains(err.Error(), "test-api-key") {
				t.Fatalf("接受不完整或未脱敏响应 %s: %v", mode, err)
			}
		})
	}
}
