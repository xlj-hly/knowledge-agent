package bootstrap

import (
	"context"
	"database/sql"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go-server/internal/app/storage"
	"go-server/internal/infra/database"
)

type observedObjectBody struct {
	io.Reader
	closed bool
}

func (b *observedObjectBody) Close() error { b.closed = true; return nil }

type injectedObjects struct {
	storage.Store
	body *observedObjectBody
	key  string
	ctx  context.Context
}

func (s *injectedObjects) Get(ctx context.Context, key string) (storage.Object, error) {
	s.key, s.ctx = key, ctx
	return storage.Object{Body: s.body, Metadata: storage.Metadata{Size: int64(len("injected-avatar-content")), ContentType: "image/png"}}, nil
}

func TestApplicationInjectsObjectStorageThroughUseCaseAndHTTP(t *testing.T) {
	// 头像读取不访问数据库；关闭自动 Ping 的连接用于验证真实装配链。
	pool, err := sql.Open("pgx", "postgres://unused:unused@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	db, err := database.Wrap(pool)
	if err != nil {
		t.Fatal(err)
	}
	resource := &observedObjectBody{Reader: strings.NewReader("injected-avatar-content")}
	store := &injectedObjects{body: resource}
	application, err := New(db, Config{PlatformHosts: []string{"platform.test"}}, WithObjectStorage(store))
	if err != nil {
		t.Fatal(err)
	}
	name := strings.Repeat("a", 32) + ".png"
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/avatars/12/34/"+name, nil)
	application.Handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "injected-avatar-content" || store.key != "avatars/12/34/"+name || !resource.closed || store.ctx != req.Context() {
		t.Fatalf("注入链路或资源责任丢失：status=%d key=%s closed=%v", w.Code, store.key, resource.closed)
	}
}

func TestApplicationRejectsMissingDatabase(t *testing.T) {
	if _, err := New(nil, Config{PlatformHosts: []string{"platform.test"}}); err == nil {
		t.Fatal("应用接受了空数据库")
	}
}
