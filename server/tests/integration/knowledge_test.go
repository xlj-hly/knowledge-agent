package saas_test

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	aidto "go-server/internal/app/ai/dto"
	kd "go-server/internal/app/knowledge/dto"
	"go-server/internal/domain/knowledge"
	kr "go-server/internal/infra/persistence/knowledge"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func knowledgeProcess(t *testing.T, s *integrationSuite) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	worked, err := s.application.Knowledge.ProcessPending(ctx)
	if err != nil || !worked {
		t.Fatalf("process persisted knowledge task: worked=%v err=%v", worked, err)
	}
}

func TestKnowledgeKeywordLifecycleAndIsolation(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "kb.alpha.test")
	s.createTenant(t, "kb.beta.test")
	a := s.login(t, "kb.alpha.test", "tenant.admin", adminPassword)
	b := s.login(t, "kb.beta.test", "tenant.admin", adminPassword)
	base := decodeSuccess[kd.Base](t, s.request("POST", "kb.alpha.test", "/api/knowledge/bases", a.Token, map[string]any{"name": "供应商资料", "indexing_mode": "keyword", "retrieval_mode": "keyword", "chunk_size": 128, "chunk_overlap": 20}))
	assertFailure(t, s.request("POST", "kb.alpha.test", "/api/knowledge/bases/"+base.ID+"/documents", a.Token, map[string]any{"title": "非法内容", "content": "原文\x00不允许"}), 400)
	doc := decodeSuccess[kd.Document](t, s.request("POST", "kb.alpha.test", "/api/knowledge/bases/"+base.ID+"/documents", a.Token, map[string]any{"title": "供应条款", "content": "海豚公司的年度设备维护费用为三千二百元。客服电话为800123。"}))
	if doc.ParseStatus != "queued" {
		t.Fatalf("new document state=%s", doc.ParseStatus)
	}
	knowledgeProcess(t, s)
	detail := decodeSuccess[kd.Document](t, s.request("GET", "kb.alpha.test", "/api/knowledge/documents/"+doc.ID, a.Token, nil))
	if detail.ParseStatus != "completed" || detail.ChunkCount < 1 {
		t.Fatalf("document not completed: %+v", detail)
	}
	result := decodeSuccess[[]kd.Hit](t, s.request("POST", "kb.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "维护费用"}))
	if len(result) != 1 || result[0].DocumentID != doc.ID || !strings.Contains(result[0].Content, "三千二百元") {
		t.Fatalf("Chinese keyword result=%+v", result)
	}
	assertFailure(t, s.request("GET", "kb.beta.test", "/api/knowledge/documents/"+doc.ID, b.Token, nil), 404)
	assertFailure(t, s.request("POST", "kb.beta.test", "/api/knowledge/bases/"+base.ID+"/search", b.Token, map[string]any{"query": "维护费用"}), 404)
	assertFailure(t, s.request("PATCH", "kb.alpha.test", "/api/knowledge/bases/"+base.ID, a.Token, map[string]any{"name": "供应商资料", "indexing_mode": "keyword", "retrieval_mode": "keyword", "chunk_size": 256, "chunk_overlap": 20}), 409)
	chunks := decodeSuccess[[]kd.Chunk](t, s.request("GET", "kb.alpha.test", "/api/knowledge/documents/"+doc.ID+"/chunks", a.Token, nil))
	assertFailure(t, s.request("PATCH", "kb.alpha.test", "/api/knowledge/chunks/"+chunks[0].ID, a.Token, map[string]any{"content": "分块\x00不允许"}), 400)
	edited := decodeSuccess[kd.Document](t, s.request("PATCH", "kb.alpha.test", "/api/knowledge/chunks/"+chunks[0].ID, a.Token, map[string]any{"content": "海豚公司的年度设备维护费用为九千元。"}))
	if edited.ParseStatus != "queued" {
		t.Fatalf("chunk editing did not queue reindex: %+v", edited)
	}
	hidden := decodeSuccess[[]kd.Hit](t, s.request("POST", "kb.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "维护费用"}))
	if len(hidden) != 0 {
		t.Fatal("old chunk still searchable during reindex")
	}
	knowledgeProcess(t, s)
	updated := decodeSuccess[[]kd.Hit](t, s.request("POST", "kb.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "维护费用"}))
	if len(updated) != 1 || !strings.Contains(updated[0].Content, "九千元") {
		t.Fatalf("edited chunks not reindexed: %+v", updated)
	}
	download := s.request("GET", "kb.alpha.test", "/api/knowledge/documents/"+doc.ID+"/download", a.Token, nil)
	if download.status != 200 || !bytes.Contains(download.body, []byte("三千二百元")) {
		t.Fatalf("chunk edit changed original: HTTP %d", download.status)
	}
	decodeSuccess[kd.Document](t, s.request("POST", "kb.alpha.test", "/api/knowledge/documents/"+doc.ID+"/reprocess", a.Token, nil))
	knowledgeProcess(t, s)
	original := decodeSuccess[[]kd.Hit](t, s.request("POST", "kb.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "维护费用"}))
	if len(original) != 1 || !strings.Contains(original[0].Content, "三千二百元") {
		t.Fatal("reprocess did not restore original chunks")
	}
	decodeSuccess[kd.OK](t, s.request("DELETE", "kb.alpha.test", "/api/knowledge/bases/"+base.ID, a.Token, nil))
	assertFailure(t, s.request("GET", "kb.alpha.test", "/api/knowledge/documents/"+doc.ID, a.Token, nil), 404)
}

func TestKnowledgeRebuildPreservesOriginalAndSourceRanges(t *testing.T) {
	s := newIntegrationSuite(t)
	tenant := s.createTenant(t, "rebuild.alpha.test")
	s.createTenant(t, "rebuild.beta.test")
	a := s.login(t, "rebuild.alpha.test", "tenant.admin", adminPassword)
	b := s.login(t, "rebuild.beta.test", "tenant.admin", adminPassword)
	base := decodeSuccess[kd.Base](t, s.request("POST", "rebuild.alpha.test", "/api/knowledge/bases", a.Token, map[string]any{"name": "员工手册", "indexing_mode": "keyword", "retrieval_mode": "keyword", "chunk_strategy": "recursive", "chunk_size": 128, "chunk_overlap": 20}))
	original := "# 员工手册\n\n## 出差报销\n\n" + strings.Repeat("交通费用每日上限一百二十元，报销须提交审批单。", 30) + "\n\n## 设备维护\n\n设备维护周期为每九十天一次。"
	doc := decodeSuccess[kd.Document](t, s.request("POST", "rebuild.alpha.test", "/api/knowledge/bases/"+base.ID+"/documents", a.Token, map[string]any{"title": "员工手册原件", "content": original}))
	knowledgeProcess(t, s)
	repo := kr.New(s.db)
	before, err := repo.Document(context.Background(), tenant.Tenant.ID, doc.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	oldChunks := decodeSuccess[[]kd.Chunk](t, s.request("GET", "rebuild.alpha.test", "/api/knowledge/documents/"+doc.ID+"/chunks", a.Token, nil))
	if before.ParseStatus != "completed" || len(oldChunks) < 2 {
		t.Fatalf("initial document was not split: status=%s chunks=%d", before.ParseStatus, len(oldChunks))
	}
	preview := decodeSuccess[[]kd.PreviewChunk](t, s.request("POST", "rebuild.alpha.test", "/api/knowledge/preview", a.Token, map[string]any{"content": original, "chunk_strategy": "heading", "chunk_size": 256, "chunk_overlap": 20}))
	unchanged := decodeSuccess[[]kd.Chunk](t, s.request("GET", "rebuild.alpha.test", "/api/knowledge/documents/"+doc.ID+"/chunks", a.Token, nil))
	if len(unchanged) != len(oldChunks) || unchanged[0].ID != oldChunks[0].ID {
		t.Fatal("preview changed persisted chunks")
	}
	input := map[string]any{"name": "员工手册", "indexing_mode": "keyword", "retrieval_mode": "keyword", "chunk_strategy": "heading", "chunk_size": 256, "chunk_overlap": 20}
	assertFailure(t, s.request("PATCH", "rebuild.alpha.test", "/api/knowledge/bases/"+base.ID, a.Token, input), 409)
	assertFailure(t, s.request("POST", "rebuild.beta.test", "/api/knowledge/bases/"+base.ID+"/rebuild", b.Token, input), 404)
	queuedBase := decodeSuccess[kd.Base](t, s.request("POST", "rebuild.alpha.test", "/api/knowledge/bases/"+base.ID+"/rebuild", a.Token, input))
	if queuedBase.ProcessingCount != 1 || queuedBase.ChunkCount != 0 || queuedBase.ChunkStrategy != "heading" || queuedBase.ChunkSize != 256 {
		t.Fatalf("rebuild did not atomically save configuration and queue work: %+v", queuedBase)
	}
	queued, err := repo.Document(context.Background(), tenant.Tenant.ID, doc.ID, false)
	if err != nil || queued.ParseStatus != "queued" || queued.Attempt == before.Attempt {
		t.Fatalf("rebuild did not replace processing ownership: %+v %v", queued, err)
	}
	hidden := decodeSuccess[[]kd.Hit](t, s.request("POST", "rebuild.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "交通费用"}))
	if len(hidden) != 0 {
		t.Fatal("rebuild exposed chunks from previous generation")
	}
	knowledgeProcess(t, s)
	current := decodeSuccess[kd.Document](t, s.request("GET", "rebuild.alpha.test", "/api/knowledge/documents/"+doc.ID, a.Token, nil))
	completeBase := decodeSuccess[kd.Base](t, s.request("GET", "rebuild.alpha.test", "/api/knowledge/bases/"+base.ID, a.Token, nil))
	chunks := decodeSuccess[[]kd.Chunk](t, s.request("GET", "rebuild.alpha.test", "/api/knowledge/documents/"+doc.ID+"/chunks", a.Token, nil))
	if current.ParseStatus != "completed" || completeBase.ProcessingCount != 0 || completeBase.ChunkCount != int64(len(chunks)) || len(chunks) != len(preview) {
		t.Fatalf("rebuild did not complete consistently: document=%+v base=%+v chunks=%d preview=%d", current, completeBase, len(chunks), len(preview))
	}
	runes, hasHeader := []rune(original), false
	for i, chunk := range chunks {
		if chunk.Start < 0 || chunk.End > len(runes) || chunk.End <= chunk.Start || string(runes[chunk.Start:chunk.End]) != chunk.Content {
			t.Fatalf("rebuilt chunk lost original Unicode source range: %+v", chunk)
		}
		if chunk.Index != i || chunk.Content != preview[i].Content || chunk.ContextHeader != preview[i].ContextHeader || chunk.Start != preview[i].Start || chunk.End != preview[i].End {
			t.Fatalf("persisted splitter differs from preview at %d: %+v vs %+v", i, chunk, preview[i])
		}
		if chunk.ContextHeader != "" {
			hasHeader = true
		}
		for _, old := range oldChunks {
			if chunk.ID == old.ID {
				t.Fatal("rebuild retained an obsolete chunk identity")
			}
		}
	}
	if !hasHeader {
		t.Fatal("heading context was not persisted")
	}
	hits := decodeSuccess[[]kd.Hit](t, s.request("POST", "rebuild.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "维护周期"}))
	if len(hits) != 1 || hits[0].DocumentID != doc.ID || !strings.Contains(hits[0].Content, "九十天") {
		t.Fatalf("rebuilt content is not searchable: %+v", hits)
	}
	download := s.request("GET", "rebuild.alpha.test", "/api/knowledge/documents/"+doc.ID+"/download", a.Token, nil)
	if download.status != http.StatusOK || string(download.body) != original {
		t.Fatalf("rebuild changed original document: HTTP %d", download.status)
	}
}

func TestKnowledgeModelConfigPermissionsAndFailureRetry(t *testing.T) {
	s := newIntegrationSuite(t)
	s.createTenant(t, "config.alpha.test")
	a := s.login(t, "config.alpha.test", "tenant.admin", adminPassword)
	member := decodeSuccess[userView](t, s.request("POST", "config.alpha.test", "/api/admin/users", a.Token, map[string]any{"username": "kb.member", "password": memberPassword, "nickname": "资料员", "role": "user"}))
	if member.ID == "" {
		t.Fatal("missing member")
	}
	u := s.login(t, "config.alpha.test", "kb.member", memberPassword)
	providers := decodeSuccess[[]kd.VectorProvider](t, s.request("GET", "config.alpha.test", "/api/knowledge/vector-providers", a.Token, nil))
	if len(providers) != 13 || providers[0].ID != "qdrant" || providers[0].Name != "Qdrant" || len(providers[0].Fields) < 3 || !strings.HasPrefix(providers[0].Logo, "data:image/") {
		t.Fatal("registered vector engine metadata is incomplete")
	}
	assertFailure(t, s.request("GET", "config.alpha.test", "/api/knowledge/vector-providers", "", nil), 401)
	assertFailure(t, s.request("POST", "config.alpha.test", "/api/knowledge/vector-stores", a.Token, map[string]any{"name": "未知引擎", "provider": "unregistered", "config": map[string]string{"endpoint": "http://127.0.0.1:1", "collection": "invalid_fixture"}}), 400)
	if rows := decodeSuccess[[]kd.VectorStore](t, s.request("GET", "config.alpha.test", "/api/knowledge/vector-stores", a.Token, nil)); len(rows) != 0 {
		t.Fatal("unknown vector engine was persisted")
	}
	modelIn := map[string]any{"name": "页面模型", "kind": "embedding", "provider": "openai", "endpoint": "http://127.0.0.1:1/v1", "model_name": "fixture", "dimension": 3, "api_key": "private-knowledge-test"}
	assertFailure(t, s.request("POST", "config.alpha.test", "/api/ai/models", u.Token, modelIn), 403)
	saved := s.request("POST", "config.alpha.test", "/api/ai/models", a.Token, modelIn)
	m := decodeSuccess[aidto.Model](t, saved)
	if m.APIKey != "private-knowledge-test" {
		t.Fatal("model configuration did not return the supplied key")
	}
	modelTest := s.request("POST", "config.alpha.test", "/api/ai/models/test", a.Token, modelIn)
	if modelTest.status != 502 || !bytes.Contains(modelTest.body, []byte(`"code":"ai_upstream_error"`)) || bytes.Contains(modelTest.body, []byte("private-knowledge-test")) {
		t.Fatalf("model test error contract: HTTP %d %s", modelTest.status, modelTest.body)
	}
	models := decodeSuccess[[]aidto.Model](t, s.request("GET", "config.alpha.test", "/api/ai/models", u.Token, nil))
	if len(models) != 1 || models[0].APIKey != m.APIKey {
		t.Fatal("member cannot read the complete authorized model configuration")
	}
	store := decodeSuccess[kd.VectorStore](t, s.request("POST", "config.alpha.test", "/api/knowledge/vector-stores", a.Token, map[string]any{"name": "页面向量连接", "provider": "qdrant", "config": map[string]string{"endpoint": "http://127.0.0.1:1", "collection": "failure_fixture"}}))
	storeTest := s.request("POST", "config.alpha.test", "/api/knowledge/vector-stores/"+store.ID+"/test", a.Token, nil)
	if storeTest.status != 502 || !bytes.Contains(storeTest.body, []byte(`"code":"knowledge_upstream_error"`)) {
		t.Fatalf("vector test error contract: HTTP %d %s", storeTest.status, storeTest.body)
	}
	base := decodeSuccess[kd.Base](t, s.request("POST", "config.alpha.test", "/api/knowledge/bases", u.Token, map[string]any{"name": "失败重试", "indexing_mode": "hybrid", "retrieval_mode": "vector", "embedding_model_id": m.ID, "vector_store_id": store.ID}))
	doc := decodeSuccess[kd.Document](t, s.request("POST", "config.alpha.test", "/api/knowledge/bases/"+base.ID+"/documents", u.Token, map[string]any{"title": "设备资料", "content": "恒星设备的保修期为十八个月。"}))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	worked, err := s.application.Knowledge.ProcessPending(ctx)
	if !worked || err == nil {
		t.Fatalf("unavailable model should fail: %v %v", worked, err)
	}
	failed := decodeSuccess[kd.Document](t, s.request("GET", "config.alpha.test", "/api/knowledge/documents/"+doc.ID, u.Token, nil))
	if failed.ParseStatus != "failed" || failed.ErrorMessage == "" || strings.Contains(failed.ErrorMessage, "private-") {
		t.Fatalf("failure state=%+v", failed)
	}
	modelIn["dimension"] = 4
	assertFailure(t, s.request("PUT", "config.alpha.test", "/api/ai/models/"+m.ID, a.Token, modelIn), 409)
	queued := decodeSuccess[kd.Document](t, s.request("POST", "config.alpha.test", "/api/knowledge/documents/"+doc.ID+"/reprocess", u.Token, nil))
	if queued.ParseStatus != "queued" || queued.ErrorMessage != "" {
		t.Fatalf("retry state=%+v", queued)
	}
}

func TestKnowledgeExpiredProcessingOwnership(t *testing.T) {
	s := newIntegrationSuite(t)
	tenant := s.createTenant(t, "claim.alpha.test")
	a := s.login(t, "claim.alpha.test", "tenant.admin", adminPassword)
	base := decodeSuccess[kd.Base](t, s.request("POST", "claim.alpha.test", "/api/knowledge/bases", a.Token, map[string]any{"name": "任务恢复", "indexing_mode": "keyword", "retrieval_mode": "keyword"}))
	doc := decodeSuccess[kd.Document](t, s.request("POST", "claim.alpha.test", "/api/knowledge/bases/"+base.ID+"/documents", a.Token, map[string]any{"title": "恢复资料", "content": "应急电池容量为四十八千瓦时。"}))
	repo := kr.New(s.db)
	ctx := context.Background()
	first, err := repo.Claim(ctx, time.Now(), time.Now().Add(-time.Minute), "first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := repo.Claim(ctx, time.Now(), time.Now().Add(time.Minute), "second")
	if err != nil {
		t.Fatal(err)
	}
	if first.Attempt == second.Attempt {
		t.Fatal("expired task ownership was not replaced")
	}
	chunks := []knowledge.Chunk{{ID: "98765432101", TenantID: tenant.Tenant.ID, BaseID: base.ID, DocumentID: doc.ID, Attempt: second.Attempt, Content: "新任务结果", CreatedAt: time.Now()}}
	ok, err := repo.Complete(ctx, second, chunks)
	if err != nil || !ok {
		t.Fatalf("new owner complete=%v err=%v", ok, err)
	}
	ok, err = repo.Complete(ctx, first, nil)
	if err != nil || ok {
		t.Fatalf("old owner must not overwrite completed result: %v %v", ok, err)
	}
	current, err := repo.Document(ctx, tenant.Tenant.ID, doc.ID, false)
	if err != nil || current.Attempt != second.Attempt || current.ChunkCount != 1 {
		t.Fatalf("completed owner overwritten: %+v %v", current, err)
	}
}

func TestKnowledgeVectorLifecycle(t *testing.T) {
	endpoint := os.Getenv("TEST_QDRANT_ENDPOINT")
	if endpoint == "" {
		t.Skip("set TEST_QDRANT_ENDPOINT for real Qdrant integration")
	}
	var block atomic.Bool
	var failCleanup atomic.Bool
	started, release := make(chan struct{}), make(chan struct{})
	embedding := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			http.NotFound(w, r)
			return
		}
		if block.CompareAndSwap(true, false) {
			close(started)
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		var in struct {
			Input []string `json:"input"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Error(err)
			http.Error(w, "invalid", 400)
			return
		}
		data := make([]map[string]any, 0, len(in.Input))
		for i := range in.Input {
			data = append(data, map[string]any{"index": i, "embedding": []float64{1, 0, 0}})
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer embedding.Close()
	upstream, err := url.Parse(endpoint)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(upstream)
	qdrant := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if failCleanup.Load() && r.Method == "POST" && strings.HasSuffix(r.URL.Path, "/points/delete") {
			http.Error(w, "temporary unavailable", 503)
			return
		}
		proxy.ServeHTTP(w, r)
	}))
	defer qdrant.Close()
	s := newIntegrationSuite(t)
	tenant := s.createTenant(t, "vector.alpha.test")
	a := s.login(t, "vector.alpha.test", "tenant.admin", adminPassword)
	m := decodeSuccess[aidto.Model](t, s.request("POST", "vector.alpha.test", "/api/ai/models", a.Token, map[string]any{"name": "受控Embedding", "kind": "embedding", "provider": "openai", "endpoint": embedding.URL + "/v1", "model_name": "test", "dimension": 3}))
	prefix := fmt.Sprintf("ktest_%s", s.schema)
	rebuiltPrefix := prefix + "_rebuilt"
	v := decodeSuccess[kd.VectorStore](t, s.request("POST", "vector.alpha.test", "/api/knowledge/vector-stores", a.Token, map[string]any{"name": "真实Qdrant", "provider": "qdrant", "config": map[string]string{"endpoint": qdrant.URL, "collection": prefix}}))
	t.Cleanup(func() {
		for _, collection := range []string{prefix, rebuiltPrefix} {
			req, _ := http.NewRequest("DELETE", strings.TrimRight(endpoint, "/")+"/collections/"+collection+"_3", nil)
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Error(err)
				continue
			}
			res.Body.Close()
			if res.StatusCode != 200 && res.StatusCode != 404 {
				t.Errorf("cleanup collection HTTP %d", res.StatusCode)
			}
		}
	})
	base := decodeSuccess[kd.Base](t, s.request("POST", "vector.alpha.test", "/api/knowledge/bases", a.Token, map[string]any{"name": "向量流程", "indexing_mode": "hybrid", "embedding_model_id": m.ID, "vector_store_id": v.ID}))
	doc := decodeSuccess[kd.Document](t, s.request("POST", "vector.alpha.test", "/api/knowledge/bases/"+base.ID+"/documents", a.Token, map[string]any{"title": "仓储规则", "content": "橙子仓储区温度须保持在十二摄氏度。"}))
	knowledgeProcess(t, s)
	search := func() []kd.Hit {
		return decodeSuccess[[]kd.Hit](t, s.request("POST", "vector.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "仓储区要求", "mode": "vector"}))
	}
	if got := search(); len(got) != 1 || got[0].DocumentID != doc.ID {
		t.Fatalf("vector hits=%+v", got)
	}
	maximum := decodeSuccess[[]kd.Hit](t, s.request("POST", "vector.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "温度", "mode": "vector", "limit": 50}))
	if len(maximum) != 1 {
		t.Fatalf("maximum valid search limit failed: %+v", maximum)
	}
	for i := 0; i < 3; i++ {
		decodeSuccess[kd.Document](t, s.request("PATCH", "vector.alpha.test", "/api/knowledge/documents/"+doc.ID, a.Token, map[string]any{"title": "仓储规则", "content": fmt.Sprintf("橙子仓储区第%d版温度规则为十二摄氏度。", i)}))
		knowledgeProcess(t, s)
		got := search()
		if len(got) != 1 || !strings.Contains(got[0].Content, fmt.Sprintf("第%d版", i)) {
			t.Fatalf("old vector shadowed current content: %+v", got)
		}
	}
	// Qdrant 的已知文档范围必须只剩当前代，不能依赖业务过滤掩盖泄漏。
	countStorePoints := func(collection string, want int) {
		t.Helper()
		payload := fmt.Sprintf(`{"filter":{"must":[{"key":"tenant_id","match":{"value":%q}},{"key":"knowledge_base_id","match":{"value":%q}},{"key":"document_id","match":{"value":%q}}]},"exact":true}`, tenant.Tenant.ID, base.ID, doc.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(endpoint, "/")+"/collections/"+collection+"_3/points/count", strings.NewReader(payload))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(io.LimitReader(res.Body, 65536))
		if err != nil {
			t.Fatal(err)
		}
		var response struct {
			Result struct {
				Count int `json:"count"`
			} `json:"result"`
		}
		if res.StatusCode != 200 || json.Unmarshal(body, &response) != nil || response.Result.Count != want {
			t.Fatalf("Qdrant point cleanup: HTTP %d count=%d want=%d", res.StatusCode, response.Result.Count, want)
		}
	}
	activePrefix := prefix
	countPoints := func(want int) { t.Helper(); countStorePoints(activePrefix, want) }
	countPoints(1)
	// 重建切换连接时，清理必须使用资料原来的连接，不能只清理新连接。
	nextStore := decodeSuccess[kd.VectorStore](t, s.request("POST", "vector.alpha.test", "/api/knowledge/vector-stores", a.Token, map[string]any{"name": "重建后的Qdrant", "provider": "qdrant", "config": map[string]string{"endpoint": qdrant.URL, "collection": rebuiltPrefix}}))
	rebuilding := decodeSuccess[kd.Base](t, s.request("POST", "vector.alpha.test", "/api/knowledge/bases/"+base.ID+"/rebuild", a.Token, map[string]any{"name": "向量流程", "indexing_mode": "hybrid", "retrieval_mode": "vector", "embedding_model_id": m.ID, "vector_store_id": nextStore.ID, "chunk_strategy": base.ChunkStrategy, "chunk_size": base.ChunkSize, "chunk_overlap": base.ChunkOverlap}))
	if rebuilding.ProcessingCount != 1 || rebuilding.VectorStoreID != nextStore.ID {
		t.Fatalf("vector rebuild not queued: %+v", rebuilding)
	}
	assertFailure(t, s.request("DELETE", "vector.alpha.test", "/api/knowledge/vector-stores/"+v.ID, a.Token, nil), 409)
	if got := search(); len(got) != 0 {
		t.Fatalf("old vector generation visible during rebuild: %+v", got)
	}
	knowledgeProcess(t, s)
	countStorePoints(prefix, 0)
	activePrefix = rebuiltPrefix
	countPoints(1)
	if got := search(); len(got) != 1 || got[0].DocumentID != doc.ID {
		t.Fatalf("rebuilt vector connection did not become searchable: %+v", got)
	}
	decodeSuccess[kd.OK](t, s.request("DELETE", "vector.alpha.test", "/api/knowledge/vector-stores/"+v.ID, a.Token, nil))
	decodeSuccess[kd.OK](t, s.request("DELETE", "vector.alpha.test", "/api/knowledge/documents/"+doc.ID, a.Token, nil))
	if got := search(); len(got) != 0 {
		t.Fatalf("deleted vectors still visible: %+v", got)
	}
	countPoints(0)
	// 模拟已经开始向量化的旧 writer 在受理删除后仍成功写入，且第一次回收返回503。
	blockingDoc := decodeSuccess[kd.Document](t, s.request("POST", "vector.alpha.test", "/api/knowledge/bases/"+base.ID+"/documents", a.Token, map[string]any{"title": "处理中删除", "content": "此资料将在模型处理期间被删除。"}))
	block.Store(true)
	finished := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_, processErr := s.application.Knowledge.ProcessPending(ctx)
		finished <- processErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("worker never reached blocked embedding")
	}
	repo := kr.New(s.db)
	before, err := repo.Document(context.Background(), tenant.Tenant.ID, blockingDoc.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	assertFailure(t, s.request("PATCH", "vector.alpha.test", "/api/knowledge/documents/"+blockingDoc.ID, a.Token, map[string]any{"title": "不允许并发编辑", "content": "新正文"}), 409)
	assertFailure(t, s.request("POST", "vector.alpha.test", "/api/knowledge/documents/"+blockingDoc.ID+"/reprocess", a.Token, nil), 409)
	decodeSuccess[kd.OK](t, s.request("DELETE", "vector.alpha.test", "/api/knowledge/documents/"+blockingDoc.ID, a.Token, nil))
	decodeSuccess[kd.OK](t, s.request("DELETE", "vector.alpha.test", "/api/knowledge/documents/"+blockingDoc.ID, a.Token, nil))
	pending, err := repo.Document(context.Background(), tenant.Tenant.ID, blockingDoc.ID, false)
	if err != nil || pending.ParseStatus != "deleting" || pending.Attempt != before.Attempt || pending.LeaseUntil == nil || !pending.LeaseUntil.Equal(*before.LeaseUntil) {
		t.Fatalf("deleting tombstone lost original lease: %+v %v", pending, err)
	}
	assertFailure(t, s.request("DELETE", "vector.alpha.test", "/api/knowledge/bases/"+base.ID, a.Token, nil), 409)
	failCleanup.Store(true)
	close(release)
	select {
	case processErr := <-finished:
		if processErr == nil {
			t.Fatal("superseded worker must report failed cleanup")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("blocked worker did not finish")
	}
	pending, err = repo.Document(context.Background(), tenant.Tenant.ID, blockingDoc.ID, false)
	if err != nil || pending.ParseStatus != "deleting" {
		t.Fatalf("failed cleanup lost tombstone: %+v %v", pending, err)
	}
	doc.ID = blockingDoc.ID
	countPoints(1)
	if _, err = s.pool.Exec(context.Background(), "UPDATE knowledge_documents SET lease_until = CURRENT_TIMESTAMP - interval '1 second' WHERE id=$1", blockingDoc.ID); err != nil {
		t.Fatal(err)
	}
	failCleanup.Store(false)
	knowledgeProcess(t, s)
	assertFailure(t, s.request("GET", "vector.alpha.test", "/api/knowledge/documents/"+blockingDoc.ID, a.Token, nil), 404)
	if got := search(); len(got) != 0 {
		t.Fatalf("late writer remained searchable after tombstone cleanup: %+v", got)
	}
	// 切换作用域以直接核实晚到旧 writer 的点已从真实Qdrant回收。
	countPoints(0)
}

func TestKnowledgeDOCXUploadDownloadAndCleanup(t *testing.T) {
	s := newIntegrationSuite(t)
	tenant := s.createTenant(t, "upload.alpha.test")
	a := s.login(t, "upload.alpha.test", "tenant.admin", adminPassword)
	base := decodeSuccess[kd.Base](t, s.request("POST", "upload.alpha.test", "/api/knowledge/bases", a.Token, map[string]any{"name": "文件流程", "indexing_mode": "keyword", "retrieval_mode": "keyword"}))
	var file bytes.Buffer
	archive := zip.NewWriter(&file)
	entry, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, err = entry.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>南风设备的校准周期为每九十天一次。</w:t></w:r></w:p></w:body></w:document>`))
	if err != nil {
		t.Fatal(err)
	}
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "南风设备.docx")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = part.Write(file.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest("POST", "http://upload.alpha.test/api/knowledge/bases/"+base.ID+"/upload", &body)
	req.Host = "upload.alpha.test"
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+a.Token)
	res := httptest.NewRecorder()
	s.handler.ServeHTTP(res, req)
	doc := decodeSuccess[kd.Document](t, apiResult{status: res.Code, body: res.Body.Bytes()})
	if doc.FileType != "application/vnd.openxmlformats-officedocument.wordprocessingml.document" {
		t.Fatalf("unexpected MIME=%s", doc.FileType)
	}
	knowledgeProcess(t, s)
	result := decodeSuccess[[]kd.Hit](t, s.request("POST", "upload.alpha.test", "/api/knowledge/bases/"+base.ID+"/search", a.Token, map[string]any{"query": "校准周期"}))
	if len(result) != 1 || !strings.Contains(result[0].Content, "九十天") {
		t.Fatalf("DOCX content not indexed: %+v", result)
	}
	download := s.request("GET", "upload.alpha.test", "/api/knowledge/documents/"+doc.ID+"/download", a.Token, nil)
	if download.status != 200 || !bytes.Equal(download.body, file.Bytes()) {
		t.Fatal("download changed original DOCX")
	}
	repo := kr.New(s.db)
	stored, err := repo.Document(context.Background(), tenant.Tenant.ID, doc.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(s.config.Storage.FilesystemRoot + "/" + stored.ObjectKey); err != nil {
		t.Fatalf("original object missing: %v", err)
	}
	decodeSuccess[kd.OK](t, s.request("DELETE", "upload.alpha.test", "/api/knowledge/documents/"+doc.ID, a.Token, nil))
	if _, err = os.Stat(s.config.Storage.FilesystemRoot + "/" + stored.ObjectKey); !os.IsNotExist(err) {
		t.Fatalf("deleted original object still exists or unexpected error: %v", err)
	}
}

func TestKnowledgeBaseMutationWaitsForDocumentCommit(t *testing.T) {
	s := newIntegrationSuite(t)
	tenant := s.createTenant(t, "concurrent.alpha.test")
	a := s.login(t, "concurrent.alpha.test", "tenant.admin", adminPassword)
	for _, operation := range []string{"update", "delete"} {
		t.Run(operation, func(t *testing.T) {
			base := decodeSuccess[kd.Base](t, s.request("POST", "concurrent.alpha.test", "/api/knowledge/bases", a.Token, map[string]any{"name": "并发边界", "indexing_mode": "keyword", "retrieval_mode": "keyword", "chunk_size": 128, "chunk_overlap": 20}))
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			tx, err := s.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			if _, err = tx.Exec(ctx, "SELECT id FROM knowledge_bases WHERE id=$1 FOR UPDATE", base.ID); err != nil {
				t.Fatal(err)
			}
			docID := "98765432102"
			if operation == "delete" {
				docID = "98765432103"
			}
			if _, err = tx.Exec(ctx, `INSERT INTO knowledge_documents(id,tenant_id,base_id,title,source,content,attempt) VALUES($1,$2,$3,'并发资料','manual','资料应保留','fixture')`, docID, tenant.Tenant.ID, base.ID); err != nil {
				t.Fatal(err)
			}
			finished := make(chan apiResult, 1)
			go func() {
				if operation == "update" {
					finished <- s.request("PATCH", "concurrent.alpha.test", "/api/knowledge/bases/"+base.ID, a.Token, map[string]any{"name": "并发边界", "indexing_mode": "keyword", "retrieval_mode": "keyword", "chunk_size": 256, "chunk_overlap": 20})
				} else {
					finished <- s.request("DELETE", "concurrent.alpha.test", "/api/knowledge/bases/"+base.ID, a.Token, nil)
				}
			}()
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
			waiting := false
			for !waiting {
				var pending bool
				if err = s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock' AND query LIKE '%knowledge_bases%')`, s.schema).Scan(&pending); err != nil {
					t.Fatal(err)
				}
				if pending {
					waiting = true
					break
				}
				select {
				case response := <-finished:
					t.Fatalf("mutation returned before pending document commit: HTTP %d %s", response.status, response.body)
				case <-ctx.Done():
					t.Fatal("mutation never waited for knowledge base lock")
				case <-ticker.C:
				}
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case response := <-finished:
				assertFailure(t, response, 409)
			case <-ctx.Done():
				t.Fatal("mutation did not finish after document commit")
			}
			current := decodeSuccess[kd.Base](t, s.request("GET", "concurrent.alpha.test", "/api/knowledge/bases/"+base.ID, a.Token, nil))
			if current.DocumentCount != 1 || current.ChunkSize != 128 {
				t.Fatalf("concurrent document boundary lost: %+v", current)
			}
			detail := decodeSuccess[kd.Document](t, s.request("GET", "concurrent.alpha.test", "/api/knowledge/documents/"+docID, a.Token, nil))
			if detail.BaseID != base.ID {
				t.Fatalf("document orphaned after mutation: %+v", detail)
			}
		})
	}
}

func TestVectorConfigurationSecretsReferencesAndTenantIsolation(t *testing.T) {
	s := newIntegrationSuite(t)
	tenant := s.createTenant(t, "vector-config.alpha.test")
	s.createTenant(t, "vector-config.beta.test")
	admin := s.login(t, "vector-config.alpha.test", "tenant.admin", adminPassword)
	other := s.login(t, "vector-config.beta.test", "tenant.admin", adminPassword)
	host, path := "vector-config.alpha.test", "/api/knowledge/vector-stores"
	input := map[string]any{"name": "模块连接", "provider": "qdrant", "config": map[string]string{"endpoint": "http://first.invalid:6333/", "collection": "scope", "api_key": "vector-private-fixture"}}
	created := s.request("POST", host, path, admin.Token, input)
	store := decodeSuccess[kd.VectorStore](t, created)
	if store.Config["api_key"] != "vector-private-fixture" {
		t.Fatal("创建响应未回显完整连接配置")
	}
	if store.Config["endpoint"] != "http://first.invalid:6333" {
		t.Fatal("连接地址规范化未保存")
	}
	repo := kr.New(s.db)
	persisted := func() knowledge.VectorStore {
		t.Helper()
		row, err := repo.VectorStore(context.Background(), tenant.Tenant.ID, store.ID, false)
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	if persisted().Config["api_key"] != "vector-private-fixture" {
		t.Fatal("凭据没有存入连接配置")
	}
	listed := decodeSuccess[[]kd.VectorStore](t, s.request("GET", host, path, admin.Token, nil))
	if len(listed) != 1 || listed[0].Config["api_key"] != "vector-private-fixture" {
		t.Fatal("配置读取未完整回显凭据")
	}
	if rows := decodeSuccess[[]kd.VectorStore](t, s.request("GET", "vector-config.beta.test", path, other.Token, nil)); len(rows) != 0 {
		t.Fatal("连接配置跨企业泄漏")
	}
	assertFailure(t, s.request("GET", host, path, "", nil), 401)
	assertFailure(t, s.request("PATCH", "vector-config.beta.test", path+"/"+store.ID, other.Token, input), 404)
	config := map[string]string{"endpoint": "http://first.invalid:6333", "collection": "scope"}
	input["config"] = config
	omitted := decodeSuccess[kd.VectorStore](t, s.request("PATCH", host, path+"/"+store.ID, admin.Token, input))
	if persisted().Config["api_key"] != "" || omitted.Config["api_key"] != "" {
		t.Fatal("省略选填凭据后仍继承旧值")
	}
	config["endpoint"] = "http://second.invalid:6333"
	changed := decodeSuccess[kd.VectorStore](t, s.request("PATCH", host, path+"/"+store.ID, admin.Token, input))
	if persisted().Config["api_key"] != "" || changed.Config["api_key"] != "" {
		t.Fatal("新服务继承了旧秘密")
	}
	config["api_key"] = "rotated-vector-private"
	rotated := decodeSuccess[kd.VectorStore](t, s.request("PATCH", host, path+"/"+store.ID, admin.Token, input))
	if persisted().Config["api_key"] != "rotated-vector-private" || rotated.Config["api_key"] != "rotated-vector-private" {
		t.Fatal("凭据轮换没有持久化并回显")
	}
	settings := decodeSuccess[aidto.Settings](t, s.request("GET", host, "/api/ai/settings", admin.Token, nil))
	settings.VectorStoreID = store.ID
	decodeSuccess[aidto.Settings](t, s.request("PUT", host, "/api/ai/settings", admin.Token, settingsInput(settings)))
	listed = decodeSuccess[[]kd.VectorStore](t, s.request("GET", host, path, admin.Token, nil))
	if len(listed) != 1 || !listed[0].Referenced || listed[0].Config["api_key"] != "rotated-vector-private" {
		t.Fatal("默认连接引用没有映射到页面")
	}
	config["endpoint"] = "http://third.invalid:6333"
	assertFailure(t, s.request("PATCH", host, path+"/"+store.ID, admin.Token, input), 409)
	assertFailure(t, s.request("DELETE", host, path+"/"+store.ID, admin.Token, nil), 409)
	if persisted().Config["endpoint"] != "http://second.invalid:6333" {
		t.Fatal("引用连接被错误迁移")
	}
	config["endpoint"] = "http://second.invalid:6333"
	config["api_key"] = ""
	updated := decodeSuccess[kd.VectorStore](t, s.request("PATCH", host, path+"/"+store.ID, admin.Token, input))
	if !updated.Referenced || updated.Config["api_key"] != "" || persisted().Config["api_key"] != "" {
		t.Fatal("被引用连接无法显式清空可选秘密")
	}
	for _, bad := range []map[string]any{{"endpoint": "http://second.invalid:6333", "collection": "scope", "api_key": nil}, {"endpoint": "http://second.invalid:6333", "collection": "scope", "unexpected": "x"}} {
		input["config"] = bad
		assertFailure(t, s.request("PATCH", host, path+"/"+store.ID, admin.Token, input), 400)
	}
	settings.VectorStoreID = ""
	decodeSuccess[aidto.Settings](t, s.request("PUT", host, "/api/ai/settings", admin.Token, settingsInput(settings)))
	decodeSuccess[kd.OK](t, s.request("DELETE", host, path+"/"+store.ID, admin.Token, nil))
	remaining := decodeSuccess[[]kd.VectorStore](t, s.request("GET", host, path, admin.Token, nil))
	if len(remaining) != 0 {
		t.Fatal("未引用连接删除后仍存在")
	}
}
