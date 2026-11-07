package usecase

import (
	"context"
	"errors"
	"testing"

	"go-server/internal/app/ai/dto"
	"go-server/internal/app/ai/port"
	"go-server/internal/domain/ai"
)

type catalogAuthorizer struct{}

func (catalogAuthorizer) Authorize(context.Context, string, string) (port.Actor, error) {
	return port.Actor{TenantID: "1", UserID: "2", Role: "admin"}, nil
}

type testCatalog struct{ calls int }

func (c *testCatalog) Providers() []dto.Provider {
	return []dto.Provider{{ID: "new-cloud", Name: "新增云"}}
}
func (c *testCatalog) ValidateModel(model ai.Model) error {
	c.calls++
	if model.Provider != "new-cloud" {
		return errors.New("not registered")
	}
	return nil
}

type testPresetCatalog struct{}

func (testPresetCatalog) Presets() dto.Presets {
	return dto.Presets{Prompts: []dto.PromptPreset{{ID: "custom", Content: "目录提供的提示词"}}}
}

func TestUsecaseReadsCatalogAndValidatesEveryProvider(t *testing.T) {
	catalog := &testCatalog{}
	service := New(Dependencies{Authorizer: catalogAuthorizer{}, Catalog: catalog})
	rows, err := service.Providers(context.Background(), dto.Identity{})
	if err != nil || len(rows) != 1 || rows[0].ID != "new-cloud" {
		t.Fatalf("providers=%v err=%v", rows, err)
	}
	m := ai.Model{Name: "测试", Provider: "new-cloud", Kind: "chat", Endpoint: "http://localhost:9000", ModelName: "custom"}
	if err := service.validateModel(m); err != nil {
		t.Fatal(err)
	}
	m.Provider = "not-registered"
	if err := service.validateModel(m); err == nil {
		t.Fatal("registration validation was bypassed")
	}
	if catalog.calls != 2 {
		t.Fatalf("catalog validation calls=%d", catalog.calls)
	}
}

func TestUsecaseReadsIndependentPresetCatalog(t *testing.T) {
	service := New(Dependencies{Authorizer: catalogAuthorizer{}, Presets: testPresetCatalog{}})
	presets, err := service.Presets(context.Background(), dto.Identity{})
	if err != nil || len(presets.Prompts) != 1 || presets.Prompts[0].ID != "custom" {
		t.Fatalf("presets=%v err=%v", presets, err)
	}
	if _, err := service.Providers(context.Background(), dto.Identity{}); err == nil {
		t.Fatal("preset catalog was used as model catalog")
	}
	service = New(Dependencies{Authorizer: catalogAuthorizer{}, Catalog: &testCatalog{}})
	if _, err := service.Presets(context.Background(), dto.Identity{}); err == nil {
		t.Fatal("model catalog was used as preset catalog")
	}
}

func TestUsecaseDoesNotFallbackWhenCatalogMissing(t *testing.T) {
	service := New(Dependencies{Authorizer: catalogAuthorizer{}})
	if _, err := service.Providers(context.Background(), dto.Identity{}); err == nil {
		t.Fatal("missing catalog hidden")
	}
	if _, err := service.Presets(context.Background(), dto.Identity{}); err == nil {
		t.Fatal("missing preset catalog hidden")
	}
	if err := service.validateModel(ai.Model{Name: "测试", Provider: "openai", Kind: "chat", Endpoint: "http://localhost:9000", ModelName: "custom"}); err == nil {
		t.Fatal("model accepted without capability catalog")
	}
}
