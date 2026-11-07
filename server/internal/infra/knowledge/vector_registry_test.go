package knowledge

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
)

type registryDriver struct {
	calls []string
	store domain.VectorStore
	scope []string
	err   error
}

func (d *registryDriver) Test(_ context.Context, store domain.VectorStore) error {
	d.calls = append(d.calls, "test")
	d.store = store
	return d.err
}
func (d *registryDriver) ReplaceDocument(_ context.Context, store domain.VectorStore, tenant, base, document, attempt string, _ []port.VectorPoint) error {
	d.calls = append(d.calls, "replace")
	d.store, d.scope = store, []string{tenant, base, document, attempt}
	return d.err
}
func (d *registryDriver) DeleteDocument(_ context.Context, store domain.VectorStore, tenant, base, document string) error {
	d.calls = append(d.calls, "delete")
	d.store, d.scope = store, []string{tenant, base, document}
	return d.err
}
func (d *registryDriver) DeleteAttempt(_ context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	d.calls = append(d.calls, "delete-attempt")
	d.store, d.scope = store, []string{tenant, base, document, attempt}
	return d.err
}
func (d *registryDriver) Search(_ context.Context, store domain.VectorStore, tenant, base string, _ []float32, _ int) ([]port.VectorHit, error) {
	d.calls = append(d.calls, "search")
	d.store, d.scope = store, []string{tenant, base}
	return []port.VectorHit{{ID: "71", Score: 0.9}}, d.err
}

func registryEntry(driver port.Vectors) Entry {
	return Entry{Definition: dto.VectorProvider{ID: "custom-engine", Name: "Custom Engine", Description: "测试自定义向量引擎", Logo: "data:image/png;base64,aWNvbg==", Protocol: "custom-rest", Fields: []dto.VectorField{{Key: "endpoint", Label: "地址", Type: "text", Required: true, Immutable: true, Default: "http://localhost:8900"}, {Key: "collection", Label: "集合", Type: "text", Required: true, Immutable: true, Default: "test"}, {Key: "api_key", Label: "密钥", Type: "password", Secret: true}}}, Driver: driver, Validate: func(domain.VectorStore) error { return nil }}
}

func TestVectorRegistryRoutesCustomEngineAndPreservesScope(t *testing.T) {
	driver := &registryDriver{}
	vectors, err := NewVectors(registryEntry(driver))
	if err != nil {
		t.Fatal(err)
	}
	store := domain.VectorStore{Name: "自定义连接", Provider: "custom-engine", Config: map[string]string{"endpoint": "http://localhost:8900", "collection": "business", "api_key": "private-key"}}
	if err := vectors.ValidateStore(store); err != nil {
		t.Fatalf("新增引擎不应修改领域品牌名单: %v", err)
	}
	ctx := context.Background()
	if err := vectors.Test(ctx, store); err != nil {
		t.Fatal(err)
	}
	if err := vectors.ReplaceDocument(ctx, store, "tenant", "base", "document", "attempt", []port.VectorPoint{{ID: "71", Vector: []float32{1, 2}}}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(driver.scope, []string{"tenant", "base", "document", "attempt"}) || !reflect.DeepEqual(driver.store, store) {
		t.Fatal("注册分派丢失企业、知识库、资料或处理代")
	}
	if err := vectors.DeleteAttempt(ctx, store, "tenant", "base", "document", "attempt"); err != nil {
		t.Fatal(err)
	}
	if err := vectors.DeleteDocument(ctx, store, "tenant", "base", "document"); err != nil {
		t.Fatal(err)
	}
	hits, err := vectors.Search(ctx, store, "tenant", "base", []float32{1, 2}, 4)
	if err != nil || len(hits) != 1 || hits[0].ID != "71" || !reflect.DeepEqual(driver.scope, []string{"tenant", "base"}) {
		t.Fatalf("检索结果或范围错误: %+v %v", hits, err)
	}
	want := []string{"test", "replace", "delete-attempt", "delete", "search"}
	if !reflect.DeepEqual(driver.calls, want) {
		t.Fatalf("没有使用注册驱动: %v", driver.calls)
	}
	driver.err = errors.New("driver unavailable")
	if err := vectors.Test(ctx, store); !errors.Is(err, driver.err) {
		t.Fatalf("驱动错误链丢失: %v", err)
	}
}

func TestVectorRegistryRejectsUnknownAndInvalidStoreBeforeDriver(t *testing.T) {
	driver := &registryDriver{}
	entry := registryEntry(driver)
	entry.Definition.Fields[1].Default = "allowed"
	entry.Validate = func(store domain.VectorStore) error {
		if store.Config["collection"] != "allowed" {
			return errors.New("collection rejected")
		}
		return nil
	}
	vectors, err := NewVectors(entry)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, provider := range []string{"not-registered", entry.Definition.ID} {
		store := domain.VectorStore{Name: "无效连接", Provider: provider, Config: map[string]string{"endpoint": "http://localhost:8900", "collection": "rejected"}}
		if err := vectors.ValidateStore(store); err == nil {
			t.Fatal("无效连接通过保存校验")
		}
		checks := []error{
			vectors.Test(ctx, store),
			vectors.ReplaceDocument(ctx, store, "t", "b", "d", "a", nil),
			vectors.DeleteDocument(ctx, store, "t", "b", "d"),
			vectors.DeleteAttempt(ctx, store, "t", "b", "d", "a"),
		}
		_, searchErr := vectors.Search(ctx, store, "t", "b", []float32{1, 2}, 2)
		checks = append(checks, searchErr)
		for _, err := range checks {
			if err == nil {
				t.Fatal("无效连接仍被派发")
			}
		}
	}
	if len(driver.calls) != 0 {
		t.Fatal("拒绝无效连接后仍调用了驱动")
	}
}

func TestVectorRegistryRegistrationAndCatalogOwnership(t *testing.T) {
	entry := registryEntry(&registryDriver{})
	for _, mutate := range []func(*Entry){
		func(e *Entry) { e.Definition.ID = "" },
		func(e *Entry) { e.Definition.ID = "INVALID ID" },
		func(e *Entry) { e.Definition.Name = "" },
		func(e *Entry) { e.Definition.Description = "" },
		func(e *Entry) { e.Definition.Protocol = "" },
		func(e *Entry) { e.Definition.Logo = "https://example.com/icon.png" },
		func(e *Entry) { e.Definition.Logo = "data:image/png;base64,broken" },
		func(e *Entry) { e.Driver = nil },
		func(e *Entry) { e.Validate = nil },
	} {
		invalid := entry
		mutate(&invalid)
		if _, err := NewVectors(invalid); err == nil {
			t.Fatalf("不完整注册通过: %+v", invalid.Definition)
		}
	}
	if _, err := NewVectors(entry, entry); err == nil {
		t.Fatal("重复引擎 ID 被接受")
	}
	vectors, err := NewVectors(entry)
	if err != nil {
		t.Fatal(err)
	}
	entry.Definition.Name = "外部修改"
	providers := vectors.Providers()
	providers[0].Name = "页面修改"
	if vectors.Providers()[0].Name != "Custom Engine" {
		t.Fatal("目录被外部修改")
	}
}

func TestBuiltinVectorsValidateQdrantProtocolConstraints(t *testing.T) {
	vectors, err := NewBuiltinVectors()
	if err != nil {
		t.Fatal(err)
	}
	store := domain.VectorStore{Name: "受控连接", Provider: "qdrant", Config: map[string]string{"endpoint": "http://localhost:6333", "collection": "business"}}
	for _, mutate := range []func(*domain.VectorStore){
		func(store *domain.VectorStore) { store.Config["endpoint"] = "http://user:secret@localhost:6333" },
		func(store *domain.VectorStore) { store.Config["endpoint"] = "http://localhost:6333?api-key=secret" },
		func(store *domain.VectorStore) { store.Config["collection"] = "business/escape" },
	} {
		invalid := store
		invalid.Config = cloneConfig(store.Config)
		mutate(&invalid)
		if err := vectors.ValidateStore(invalid); err == nil {
			t.Fatal("Qdrant 专有地址或集合限制未生效")
		}
	}
	if err := vectors.ValidateStore(store); err != nil {
		t.Fatalf("合法 Qdrant 连接被拒绝: %v", err)
	}
}

func TestVectorConfigUsesCompleteSubmittedValues(t *testing.T) {
	vectors, err := NewVectors(registryEntry(&registryDriver{}))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, endpoint, collection string
		config                     map[string]string
		want                       string
	}{
		{name: "完整回填", endpoint: "http://first.test", collection: "one", config: map[string]string{"api_key": "saved-private"}, want: "saved-private"},
		{name: "省略", endpoint: "http://first.test", collection: "one"},
		{name: "改地址", endpoint: "http://second.test", collection: "one"},
		{name: "改索引", endpoint: "http://first.test", collection: "two"},
		{name: "显式清空", endpoint: "http://first.test", collection: "one", config: map[string]string{"api_key": ""}},
		{name: "替换", endpoint: "http://first.test", collection: "one", config: map[string]string{"api_key": "new-private"}, want: "new-private"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			input := domain.VectorStore{Name: "新名称", Provider: "custom-engine", Config: map[string]string{"endpoint": test.endpoint, "collection": test.collection}}
			for key, value := range test.config {
				input.Config[key] = value
			}
			next, err := vectors.PrepareStore(input)
			if err != nil {
				t.Fatal(err)
			}
			if next.Config["api_key"] != test.want {
				t.Fatal("密钥没有采用本次完整配置")
			}
			next.Config["endpoint"] = "changed"
			next.Config["api_key"] = "changed"
			if input.Config["endpoint"] != test.endpoint || input.Config["api_key"] != test.want {
				t.Fatal("准备配置修改了调用方map")
			}
		})
	}
}

func TestVectorSchemaRejectsUnknownKeysAndInvalidPrimitiveStrings(t *testing.T) {
	entry := registryEntry(&registryDriver{})
	entry.Definition.Fields = append(entry.Definition.Fields,
		dto.VectorField{Key: "port", Label: "端口", Type: "number"},
		dto.VectorField{Key: "tls", Label: "TLS", Type: "boolean"},
		dto.VectorField{Key: "metric", Label: "度量", Type: "select", Options: []dto.VectorFieldOption{{Value: "cosine", Label: "Cosine"}}},
	)
	vectors, err := NewVectors(entry)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []map[string]string{{"port": "NaN"}, {"port": "+Inf"}, {"port": "abc"}, {"tls": "1"}, {"metric": "unknown"}, {"unregistered": "value"}} {
		config := map[string]string{"endpoint": "http://localhost", "collection": "one"}
		for k, value := range bad {
			config[k] = value
		}
		if err := vectors.ValidateInput(domain.VectorStore{Name: "连接", Provider: "custom-engine", Config: config}); err == nil {
			t.Fatalf("非法字段值通过: %+v", bad)
		}
	}
	for _, config := range []map[string]string{nil, {}, {"endpoint": "http://localhost"}} {
		if err := vectors.ValidateInput(domain.VectorStore{Name: "连接", Provider: "custom-engine", Config: config}); err == nil {
			t.Fatal("缺失普通必填配置被接受")
		}
	}
	entry.Definition.Fields[2].Required = true
	vectors, err = NewVectors(entry)
	if err != nil {
		t.Fatal(err)
	}
	candidate := domain.VectorStore{Name: "连接", Provider: "custom-engine", Config: map[string]string{"endpoint": "http://localhost", "collection": "one"}}
	if err := vectors.ValidateInput(candidate); err == nil {
		t.Fatal("输入缺少必填凭据仍被接受")
	}
	if _, err := vectors.PrepareStore(candidate); err == nil {
		t.Fatal("完整配置缺少必填凭据仍被允许")
	}
	candidate.Config["api_key"] = ""
	if err := vectors.ValidateInput(candidate); err == nil {
		t.Fatal("必填凭据允许显式清空")
	}
	candidate.Config["api_key"] = "supplied"
	if err := vectors.ValidateInput(candidate); err != nil {
		t.Fatalf("显式提交必填凭据被拒绝: %v", err)
	}
	if prepared, err := vectors.PrepareStore(candidate); err != nil || prepared.Config["api_key"] != "supplied" {
		t.Fatal("必填凭据没有按提交值准备")
	}
}

func TestVectorCatalogDeepCopyProtectsFieldDefinitions(t *testing.T) {
	entry := registryEntry(&registryDriver{})
	entry.Definition.Fields = append(entry.Definition.Fields, dto.VectorField{Key: "metric", Label: "度量", Type: "select", Options: []dto.VectorFieldOption{{Value: "cosine", Label: "Cosine"}}})
	vectors, err := NewVectors(entry)
	if err != nil {
		t.Fatal(err)
	}
	entry.Definition.Fields[0].Key = "changed"
	entries := vectors.Providers()
	entries[0].Fields[3].Options[0].Value = "changed"
	got := vectors.Providers()
	if got[0].Fields[0].Key != "endpoint" || got[0].Fields[3].Options[0].Value != "cosine" {
		t.Fatal("外部修改污染了注册契约")
	}
	if got[0].Fields[0].Options == nil {
		t.Fatal("空选项应序列化为空数组")
	}
}
