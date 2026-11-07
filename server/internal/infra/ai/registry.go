// Package ai 注册模型服务商与协议，并实现发现、检测、生成及文本向量化。
package ai

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"go-server/internal/app/ai/dto"
	chatport "go-server/internal/app/chat/port"
	domain "go-server/internal/domain/ai"
	saas "go-server/internal/domain/saas"
	"go-server/internal/infra/ai/providers"
)

// Driver 实现一个模型协议；品牌差异通过 Definition 明确提供。
// 注册后同一驱动实例会被多个请求共享，实现在调用期间须保持配置只读并支持并发。
type Driver interface {
	Protocol() string
	Kinds() []string
	Discover(context.Context, providers.Definition, domain.Model) ([]dto.ModelOption, error)
	Test(context.Context, providers.Definition, domain.Model) (int, error)
	Stream(context.Context, providers.Definition, domain.Model, chatport.ChatRequest, func(string) error) error
	EmbedBatch(context.Context, providers.Definition, domain.Model, []string) ([][]float32, error)
}

// Registry 在构造时校验并复制目录，此后只读；无进程全局注册或隐式初始化。
type Registry struct {
	definitions []providers.Definition
	byID        map[string]int
	drivers     map[string]Driver
}

// NewRegistry 显式组装服务商与协议，拒绝不完整或互相矛盾的注册。
func NewRegistry(definitions []providers.Definition, drivers ...Driver) (*Registry, error) {
	r := &Registry{byID: make(map[string]int), drivers: make(map[string]Driver)}
	for _, driver := range drivers {
		if driver == nil || driver.Protocol() == "" {
			return nil, errors.New("模型协议驱动无效")
		}
		if _, ok := r.drivers[driver.Protocol()]; ok {
			return nil, fmt.Errorf("模型协议重复: %s", driver.Protocol())
		}
		r.drivers[driver.Protocol()] = driver
	}
	for _, definition := range definitions {
		if _, ok := r.byID[definition.ID]; ok {
			return nil, fmt.Errorf("服务商重复: %s", definition.ID)
		}
		if strings.TrimSpace(definition.ID) == "" || len(definition.ID) > 64 || definition.Name == "" || definition.Description == "" || definition.Logo == "" {
			return nil, errors.New("服务商定义不完整")
		}
		if err := validateLogo(definition.Logo); err != nil {
			return nil, fmt.Errorf("服务商 %s 的图形无效: %w", definition.ID, err)
		}
		driver, ok := r.drivers[definition.Protocol]
		if !ok {
			return nil, fmt.Errorf("服务商 %s 未注册协议 %s", definition.ID, definition.Protocol)
		}
		if err := domain.ValidateEndpoint(definition.Endpoint, true); err != nil {
			return nil, fmt.Errorf("服务商 %s 默认地址无效: %w", definition.ID, err)
		}
		seen := map[string]bool{}
		for _, kind := range definition.Kinds {
			if (kind != "chat" && kind != "embedding") || seen[kind] || !contains(driver.Kinds(), kind) {
				return nil, fmt.Errorf("服务商 %s 的能力与驱动不匹配", definition.ID)
			}
			seen[kind] = true
		}
		if len(seen) == 0 {
			return nil, fmt.Errorf("服务商 %s 没有能力", definition.ID)
		}
		for _, model := range definition.Models {
			if model.ID == "" || !seen[model.Kind] || model.Kind == "embedding" && (model.Dimension < 1 || model.Dimension > 65536) || model.Kind == "chat" && model.Dimension != 0 {
				return nil, fmt.Errorf("服务商 %s 的候选模型无效", definition.ID)
			}
		}
		c := definition.Compatibility
		for _, path := range []string{c.RootPath, c.DiscoveryPath, c.EmbeddingDiscoveryPath} {
			if path != "" && (!strings.HasPrefix(path, "/") || strings.ContainsAny(path, "?#")) {
				return nil, fmt.Errorf("服务商 %s 的协议路径无效", definition.ID)
			}
		}
		if definition.Protocol == "openai" && c.TokenField != "max_tokens" && c.TokenField != "max_completion_tokens" {
			return nil, fmt.Errorf("服务商 %s 的输出参数无效", definition.ID)
		}
		r.byID[definition.ID] = len(r.definitions)
		r.definitions = append(r.definitions, cloneDefinition(definition))
	}
	return r, nil
}

func validateLogo(value string) error {
	if len(value) > 256*1024 {
		return errors.New("图形超过 256 KiB")
	}
	prefix, payload, ok := strings.Cut(value, ";base64,")
	if !ok || (prefix != "data:image/svg+xml" && prefix != "data:image/png" && prefix != "data:image/jpeg") {
		return errors.New("图形必须为受控 SVG、PNG 或 JPEG data URI")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(data) == 0 {
		return errors.New("图形 base64 内容无效")
	}
	return nil
}

// NewBuiltinRegistry 创建内置目录与现有协议的独立实例。
func NewBuiltinRegistry() (*Registry, error) {
	definitions, err := providers.Builtins()
	if err != nil {
		return nil, err
	}
	return NewRegistry(definitions, NewOpenAIDriver(), NewOllamaDriver())
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}
func cloneDefinition(value providers.Definition) providers.Definition {
	value.Kinds = append([]string{}, value.Kinds...)
	value.Models = append([]providers.Model{}, value.Models...)
	return value
}
func (r *Registry) resolve(provider, kind string) (providers.Definition, Driver, error) {
	if r == nil {
		return providers.Definition{}, nil, errors.New("AI 能力目录未配置")
	}
	index, ok := r.byID[provider]
	if !ok {
		return providers.Definition{}, nil, saas.Invalid("服务商未注册")
	}
	definition := r.definitions[index]
	if !contains(definition.Kinds, kind) {
		return providers.Definition{}, nil, saas.Invalid("服务商不支持所选模型用途")
	}
	return cloneDefinition(definition), r.drivers[definition.Protocol], nil
}

// ValidateModel 将领域结构校验与运行时能力校验组合，领域层不识别服务商品牌。
func (r *Registry) ValidateModel(model domain.Model) error {
	if err := model.Validate(); err != nil {
		return err
	}
	_, _, err := r.resolve(model.Provider, model.Kind)
	return err
}

// Providers 返回独立的公开目录，调用方修改切片不会影响后续请求。
func (r *Registry) Providers() []dto.Provider {
	out := make([]dto.Provider, 0, len(r.definitions))
	for _, definition := range r.definitions {
		models := make([]dto.ModelOption, 0, len(definition.Models))
		for _, m := range definition.Models {
			models = append(models, dto.ModelOption{ID: m.ID, Name: m.ID, Kind: m.Kind, Dimension: m.Dimension})
		}
		out = append(out, dto.Provider{ID: definition.ID, Name: definition.Name, Description: definition.Description, Logo: definition.Logo, Protocol: definition.Protocol, Endpoint: definition.Endpoint, Kinds: append([]string{}, definition.Kinds...), Models: models, CredentialRequired: definition.CredentialRequired, Discovery: definition.Compatibility.DiscoveryPath != ""})
	}
	return out
}
