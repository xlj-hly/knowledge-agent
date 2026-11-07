package knowledge

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

func TestChromaRealLifecycle(t *testing.T) {
	endpoint := os.Getenv("TEST_CHROMA_ENDPOINT")
	if endpoint == "" {
		t.Skip("设置 TEST_CHROMA_ENDPOINT 后运行真实 Chroma 生命周期")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	prefix := "agent_test_" + strconv.FormatInt(time.Now().UnixNano(), 36)
	store := domain.VectorStore{Provider: "chroma", Config: map[string]string{"endpoint": endpoint, "tenant": "default_tenant", "database": "default_database", "collection": prefix}}
	driver := ChromaEntries()[0].Driver.(*chromaDriver)
	// 仅回收本次随机命名的集合；即使业务断言失败也执行清理。
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := driver.request(cleanup, store, "DELETE", "/collections/"+prefix+"_2", nil, nil); err != nil {
			t.Errorf("cleanup %s: %v", prefix, err)
		}
	})
	if err := driver.Test(ctx, store); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		tenant, base, doc, attempt, id string
		vector                         []float32
	}{
		{"tenant-a", "base-a", "doc-a", "old", "17", []float32{1, 0}},
		{"tenant-a", "base-a", "doc-a", "new", "17", []float32{1, 0}},
		{"tenant-b", "base-a", "doc-a", "old", "17", []float32{1, 0}},
		{"tenant-a", "base-b", "doc-a", "old", "17", []float32{1, 0}},
		{"tenant-a", "base-a", "doc-b", "old", "18", []float32{0, 1}},
	} {
		if err := driver.ReplaceDocument(ctx, store, row.tenant, row.base, row.doc, row.attempt, []port.VectorPoint{{ID: row.id, Vector: row.vector}}); err != nil {
			t.Fatal(err)
		}
	}
	hits, err := driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 10)
	if err != nil || len(hits) != 3 || hits[0].ID != "17" || hits[0].Score < .99 {
		t.Fatalf("initial=%v %v", hits, err)
	}
	if err := driver.DeleteAttempt(ctx, store, "tenant-a", "base-a", "doc-a", "old"); err != nil {
		t.Fatal(err)
	}
	hits, err = driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 10)
	if err != nil || len(hits) != 2 || hits[0].ID != "17" {
		t.Fatalf("new generation lost=%v %v", hits, err)
	}
	if err := driver.DeleteDocument(ctx, store, "tenant-a", "base-a", "doc-a"); err != nil {
		t.Fatal(err)
	}
	hits, err = driver.Search(ctx, store, "tenant-a", "base-a", []float32{1, 0}, 10)
	if err != nil || len(hits) != 1 || hits[0].ID != "18" {
		t.Fatalf("delete=%v %v", hits, err)
	}
	for _, scope := range [][2]string{{"tenant-b", "base-a"}, {"tenant-a", "base-b"}} {
		hits, err = driver.Search(ctx, store, scope[0], scope[1], []float32{1, 0}, 10)
		if err != nil || len(hits) != 1 || hits[0].ID != "17" {
			t.Fatalf("foreign scope changed=%v %v", hits, err)
		}
	}
}
