package usecase

import (
	"context"
	"errors"
	"maps"
	"testing"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	"go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
)

type catalogAuthorization struct{ err error }

func (a catalogAuthorization) Authorize(context.Context, string, string) (port.Actor, error) {
	return port.Actor{TenantID: "10", UserID: "20", Role: "admin"}, a.err
}

type testVectorCatalog struct {
	validationError error
	listCalls       int
	validateCalls   int
}

func (c *testVectorCatalog) Providers() []dto.VectorProvider {
	c.listCalls++
	return []dto.VectorProvider{{ID: "test-engine", Name: "测试引擎"}}
}

func (c *testVectorCatalog) ValidateStore(store knowledge.VectorStore) error {
	c.validateCalls++
	if c.validationError != nil {
		return c.validationError
	}
	return store.Validate()
}

func (c *testVectorCatalog) ValidateInput(store knowledge.VectorStore) error {
	return c.ValidateStore(store)
}
func (c *testVectorCatalog) PrepareStore(store knowledge.VectorStore) (knowledge.VectorStore, error) {
	store.Config = maps.Clone(store.Config)
	return store, c.ValidateStore(store)
}
func (c *testVectorCatalog) ImmutableChanged(old, next knowledge.VectorStore) bool {
	return old.Provider != next.Provider || old.Config["endpoint"] != next.Config["endpoint"] || old.Config["collection"] != next.Config["collection"]
}

type vectorConfigRepository struct {
	knowledge.Repository
	transactions int
	saved        []knowledge.VectorStore
}

func (r *vectorConfigRepository) Transaction(ctx context.Context, fn func(knowledge.Repository) error) error {
	r.transactions++
	return fn(r)
}

func (r *vectorConfigRepository) SaveVectorStore(_ context.Context, store knowledge.VectorStore, _ bool) error {
	store.Config = maps.Clone(store.Config)
	r.saved = append(r.saved, store)
	return nil
}

func (r *vectorConfigRepository) VectorStores(_ context.Context, tenant string) ([]knowledge.VectorStore, error) {
	if len(r.saved) > 0 && r.saved[len(r.saved)-1].TenantID == tenant {
		return []knowledge.VectorStore{r.saved[len(r.saved)-1]}, nil
	}
	return nil, nil
}

func (r *vectorConfigRepository) VectorStore(_ context.Context, tenant, id string, _ bool) (knowledge.VectorStore, error) {
	if len(r.saved) > 0 {
		store := r.saved[len(r.saved)-1]
		if store.TenantID == tenant && store.ID == id {
			return store, nil
		}
	}
	return knowledge.VectorStore{}, saas.NotFound()
}

func (r *vectorConfigRepository) VectorStoreReferences(context.Context, string, string) (int64, error) {
	return 0, nil
}

func TestVectorCatalogRequiresAuthenticatedIdentity(t *testing.T) {
	catalog := &testVectorCatalog{}
	denied := saas.Forbidden()
	s := New(Dependencies{Authorizer: catalogAuthorization{err: denied}, Catalog: catalog})
	if _, err := s.VectorProviders(context.Background(), dto.Identity{}); !errors.Is(err, denied) || catalog.listCalls != 0 {
		t.Fatalf("拒绝身份后仍读取目录: calls=%d err=%v", catalog.listCalls, err)
	}
	s.deps.Authorizer = catalogAuthorization{}
	providers, err := s.VectorProviders(context.Background(), dto.Identity{})
	if err != nil || len(providers) != 1 || providers[0].ID != "test-engine" || catalog.listCalls != 1 {
		t.Fatalf("有效身份没有取得已注册目录: %+v %v", providers, err)
	}
}

func TestVectorSaveValidatesRegisteredEngineBeforeWrite(t *testing.T) {
	repo := &vectorConfigRepository{}
	rejected := saas.Invalid("未注册引擎")
	catalog := &testVectorCatalog{validationError: rejected}
	s := New(Dependencies{Repository: repo, Authorizer: catalogAuthorization{}, Models: testModels{}, Catalog: catalog})
	input := dto.VectorStoreInput{Name: "自定义连接", Provider: "test-engine", Config: map[string]string{"endpoint": "http://localhost:8900", "collection": "namespace"}}
	if _, err := s.SaveVectorStore(context.Background(), dto.Identity{}, "", input); !errors.Is(err, rejected) || repo.transactions != 0 || len(repo.saved) != 0 {
		t.Fatalf("未注册引擎触发了数据库写入: tx=%d saved=%d err=%v", repo.transactions, len(repo.saved), err)
	}
	catalog.validationError = nil
	output, err := s.SaveVectorStore(context.Background(), dto.Identity{}, "", input)
	if err != nil || output.Provider != "test-engine" || len(repo.saved) != 1 || repo.saved[0].TenantID != "10" {
		t.Fatalf("注册引擎未按当前企业保存: %+v %+v %v", output, repo.saved, err)
	}
	if catalog.validateCalls < 2 {
		t.Fatal("保存没有使用注册目录校验")
	}
}

func TestVectorCatalogMissingDependencyReturnsError(t *testing.T) {
	s := New(Dependencies{Authorizer: catalogAuthorization{}})
	if _, err := s.VectorProviders(context.Background(), dto.Identity{}); err == nil {
		t.Fatal("未组装目录被伪装为空供应商列表")
	}
	if _, err := s.SaveVectorStore(context.Background(), dto.Identity{}, "", dto.VectorStoreInput{}); err == nil {
		t.Fatal("未组装目录仍允许保存")
	}
}

func TestVectorConfigurationReadAndReplacement(t *testing.T) {
	repo := &vectorConfigRepository{}
	s := New(Dependencies{Repository: repo, Authorizer: catalogAuthorization{}, Models: testModels{}, Catalog: &testVectorCatalog{}})
	input := dto.VectorStoreInput{Name: "完整配置", Provider: "test-engine", Config: map[string]string{"endpoint": "http://localhost:8900", "collection": "namespace", "api_key": "original-key"}}
	created, err := s.SaveVectorStore(context.Background(), dto.Identity{}, "", input)
	if err != nil || created.Config["api_key"] != "original-key" {
		t.Fatalf("保存响应没有回显完整配置: %v", err)
	}
	created.Config["api_key"] = "caller-mutation"
	listed, err := s.VectorStores(context.Background(), dto.Identity{})
	if err != nil || len(listed) != 1 || listed[0].Config["api_key"] != "original-key" {
		t.Fatalf("读取没有返回完整配置或保存结果污染了存储: %v", err)
	}
	listed[0].Config["api_key"] = "list-mutation"
	if repo.saved[0].Config["api_key"] != "original-key" || input.Config["api_key"] != "original-key" {
		t.Fatal("配置读取复用了持久化或输入map")
	}
	for _, value := range []struct {
		key  string
		omit bool
	}{{key: "rotated-key"}, {}, {key: "restored-key"}, {omit: true}} {
		if value.omit {
			delete(input.Config, "api_key")
		} else {
			input.Config["api_key"] = value.key
		}
		updated, err := s.SaveVectorStore(context.Background(), dto.Identity{}, created.ID, input)
		if err != nil || updated.Config["api_key"] != value.key {
			t.Fatalf("更新没有使用本次完整配置: %v", err)
		}
		rows, err := s.VectorStores(context.Background(), dto.Identity{})
		if err != nil || len(rows) != 1 || rows[0].Config["api_key"] != value.key {
			t.Fatalf("后续读取仍保留旧凭据: %v", err)
		}
	}
}
