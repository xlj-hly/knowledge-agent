package knowledge

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

// TestQdrantLifecycle 仅在显式指定测试服务时执行，每次使用独立集合并清理。
func TestQdrantLifecycle(t *testing.T) {
	endpoint := os.Getenv("TEST_QDRANT_ENDPOINT")
	if endpoint == "" {
		t.Skip("设置 TEST_QDRANT_ENDPOINT 后执行真实 Qdrant 生命周期测试")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	v, err := NewBuiltinVectors()
	if err != nil {
		t.Fatal(err)
	}
	driver := newQdrantDriver()
	store := domain.VectorStore{Name: "真实测试连接", Provider: "qdrant", Config: map[string]string{"endpoint": endpoint, "api_key": os.Getenv("TEST_QDRANT_API_KEY"), "collection": fmt.Sprintf("agent_test_%d", time.Now().UnixNano())}}
	if err := v.Test(ctx, store); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		status, err := driver.request(cleanupCtx, store, http.MethodDelete, "/collections/"+collectionName(store, 2), nil, nil)
		if err != nil && status != http.StatusNotFound {
			t.Errorf("清理测试集合 %s 失败: %v", store.Config["collection"], err)
		}
	})
	put := func(tenant, base, doc, attempt, id string, vector []float32) {
		t.Helper()
		if err := v.ReplaceDocument(ctx, store, tenant, base, doc, attempt, []port.VectorPoint{{ID: id, Vector: vector}}); err != nil {
			t.Fatal(err)
		}
	}
	search := func(tenant, base string) []port.VectorHit {
		t.Helper()
		hits, err := v.Search(ctx, store, tenant, base, []float32{1, 0}, 10)
		if err != nil {
			t.Fatal(err)
		}
		return hits
	}
	put("tenant-a", "base-a", "doc-a", "old", "101", []float32{1, 0})
	put("tenant-a", "base-a", "doc-a", "new", "102", []float32{0.9, 0.1})
	put("tenant-b", "base-a", "doc-a", "new", "103", []float32{1, 0})
	put("tenant-a", "base-b", "doc-a", "new", "104", []float32{1, 0})
	if err := driver.ensureCollection(ctx, store, 3); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		status, err := driver.request(cleanupCtx, store, http.MethodDelete, "/collections/"+collectionName(store, 3), nil, nil)
		if err != nil && status != http.StatusNotFound {
			t.Errorf("清理测试维度集合失败: %v", err)
		}
	})
	hits := search("tenant-a", "base-a")
	if len(hits) != 2 || hits[0].ID != "101" {
		t.Fatalf("tenant/base filter or ranking incorrect: %v", hits)
	}
	if err := v.DeleteAttempt(ctx, store, "tenant-a", "base-a", "doc-a", "old"); err != nil {
		t.Fatal(err)
	}
	hits = search("tenant-a", "base-a")
	if len(hits) != 1 || hits[0].ID != "102" {
		t.Fatalf("deleting old attempt affected new generation: %v", hits)
	}
	if err := v.DeleteDocument(ctx, store, "tenant-a", "base-a", "doc-a"); err != nil {
		t.Fatal(err)
	}
	if hits = search("tenant-a", "base-a"); len(hits) != 0 {
		t.Fatalf("document vectors remain: %v", hits)
	}
	if hits = search("tenant-b", "base-a"); len(hits) != 1 || hits[0].ID != "103" {
		t.Fatalf("other tenant vectors changed: %v", hits)
	}
	if hits = search("tenant-a", "base-b"); len(hits) != 1 || hits[0].ID != "104" {
		t.Fatalf("other base vectors changed: %v", hits)
	}
	if err := v.DeleteDocument(ctx, store, "tenant-a", "base-a", "doc-a"); err != nil {
		t.Fatalf("delete must be idempotent: %v", err)
	}
}
