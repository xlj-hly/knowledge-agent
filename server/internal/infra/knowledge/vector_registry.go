package knowledge

import (
	"context"
	"encoding/base64"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"go-server/internal/app/knowledge/dto"
	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	saas "go-server/internal/domain/saas"
)

// Entry 将展示预设、纯配置校验与实际驱动显式关联，注册后必须支持并发读取。
// Normalize 仅规范化配置，不得联网或修改传入的 map。
type Entry struct {
	Definition dto.VectorProvider
	Driver     port.Vectors
	Validate   func(domain.VectorStore) error
	Normalize  func(map[string]string) (map[string]string, error)
}

// Vectors 是启动时组装后只读的引擎目录，不缓存企业连接或凭据。
type Vectors struct {
	entries   map[string]Entry
	providers []dto.VectorProvider
}

// NewVectors 按显式顺序构建目录，拒绝重复标识和不完整的字段契约。
func NewVectors(entries ...Entry) (*Vectors, error) {
	registry := &Vectors{entries: make(map[string]Entry, len(entries)), providers: make([]dto.VectorProvider, 0, len(entries))}
	identifier := regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	for _, entry := range entries {
		def := cloneProvider(entry.Definition)
		if !identifier.MatchString(def.ID) || !identifier.MatchString(def.Protocol) || strings.TrimSpace(def.Name) == "" || strings.TrimSpace(def.Description) == "" || !validVectorLogo(def.Logo) || entry.Driver == nil || entry.Validate == nil || len(def.Fields) == 0 || len(def.Fields) > 64 {
			return nil, fmt.Errorf("向量引擎注册定义不完整: %q", def.ID)
		}
		if _, exists := registry.entries[def.ID]; exists {
			return nil, fmt.Errorf("向量引擎重复注册: %q", def.ID)
		}
		fields := make(map[string]bool, len(def.Fields))
		for _, field := range def.Fields {
			if !regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`).MatchString(field.Key) || strings.TrimSpace(field.Label) == "" || fields[field.Key] || (field.Secret && field.Default != "") {
				return nil, fmt.Errorf("向量引擎字段定义无效: %q/%q", def.ID, field.Key)
			}
			fields[field.Key] = true
			if !validFieldType(field.Type) {
				return nil, fmt.Errorf("向量引擎字段类型无效: %q/%q", def.ID, field.Key)
			}
			if (field.Type == "select") != (len(field.Options) > 0) {
				return nil, fmt.Errorf("向量引擎字段选项无效: %q/%q", def.ID, field.Key)
			}
			options := make(map[string]bool, len(field.Options))
			for _, option := range field.Options {
				if option.Value == "" || option.Label == "" || options[option.Value] {
					return nil, fmt.Errorf("向量引擎重复或无效选项: %q/%q", def.ID, field.Key)
				}
				options[option.Value] = true
			}
			if field.Default != "" {
				if err := validateField(field, field.Default); err != nil {
					return nil, fmt.Errorf("向量引擎默认值无效: %q/%q", def.ID, field.Key)
				}
			}
		}
		entry.Definition = def
		registry.entries[def.ID] = entry
		registry.providers = append(registry.providers, def)
	}
	return registry, nil
}

func validVectorLogo(logo string) bool {
	if len(logo) > 256*1024 {
		return false
	}
	for _, prefix := range []string{"data:image/png;base64,", "data:image/jpeg;base64,", "data:image/svg+xml;base64,"} {
		if strings.HasPrefix(logo, prefix) {
			image, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(logo, prefix))
			return err == nil && len(image) > 0
		}
	}
	return false
}
func cloneConfig(config map[string]string) map[string]string {
	if config == nil {
		return nil
	}
	copy := make(map[string]string, len(config))
	for key, value := range config {
		copy[key] = value
	}
	return copy
}
func cloneProvider(provider dto.VectorProvider) dto.VectorProvider {
	fields := make([]dto.VectorField, len(provider.Fields))
	for i, field := range provider.Fields {
		field.Options = append([]dto.VectorFieldOption{}, field.Options...)
		fields[i] = field
	}
	provider.Fields = fields
	return provider
}

// Providers 返回深拷贝，调用方不能改变注册实例的字段和选项。
func (v *Vectors) Providers() []dto.VectorProvider {
	result := make([]dto.VectorProvider, len(v.providers))
	for i, provider := range v.providers {
		result[i] = cloneProvider(provider)
	}
	return result
}
func validFieldType(kind string) bool {
	switch kind {
	case "text", "password", "number", "select", "boolean":
		return true
	}
	return false
}
func validateField(field dto.VectorField, value string) error {
	if value == "" {
		return nil
	}
	switch field.Type {
	case "number":
		number, err := strconv.ParseFloat(value, 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return saas.Invalid("向量配置数字字段无效: " + field.Label)
		}
	case "boolean":
		if value != "true" && value != "false" {
			return saas.Invalid("向量配置布尔字段无效: " + field.Label)
		}
	case "select":
		for _, option := range field.Options {
			if option.Value == value {
				return nil
			}
		}
		return saas.Invalid("向量配置选项无效: " + field.Label)
	}
	return nil
}
func (v *Vectors) entry(store domain.VectorStore) (Entry, error) {
	if err := store.Validate(); err != nil {
		return Entry{}, err
	}
	entry, exists := v.entries[store.Provider]
	if !exists {
		return Entry{}, saas.Invalid("向量引擎未注册或不受支持")
	}
	return entry, nil
}
func validateConfig(entry Entry, config map[string]string) error {
	allowed := make(map[string]bool, len(entry.Definition.Fields))
	for _, field := range entry.Definition.Fields {
		allowed[field.Key] = true
		value := config[field.Key]
		if field.Required && strings.TrimSpace(value) == "" {
			return saas.Invalid("向量配置必填字段缺失: " + field.Label)
		}
		if err := validateField(field, value); err != nil {
			return err
		}
	}
	for key := range config {
		if !allowed[key] {
			return saas.Invalid("向量配置包含未声明字段: " + key)
		}
	}
	return nil
}
func normalize(entry Entry, store domain.VectorStore) (domain.VectorStore, error) {
	store.Config = cloneConfig(store.Config)
	if entry.Normalize != nil {
		config, err := entry.Normalize(store.Config)
		if err != nil {
			return domain.VectorStore{}, err
		}
		store.Config = cloneConfig(config)
	}
	return store, nil
}

// ValidateInput 在写事务前检查完整字段形状和必填配置。
func (v *Vectors) ValidateInput(store domain.VectorStore) error {
	entry, err := v.entry(store)
	if err != nil {
		return err
	}
	return validateConfig(entry, store.Config)
}

// PrepareStore 复制并规范化完整配置，省略的选填字段保持缺省，不从旧记录继承。
func (v *Vectors) PrepareStore(candidate domain.VectorStore) (domain.VectorStore, error) {
	entry, err := v.entry(candidate)
	if err != nil {
		return domain.VectorStore{}, err
	}
	if err := validateConfig(entry, candidate.Config); err != nil {
		return domain.VectorStore{}, err
	}
	candidate, err = normalize(entry, candidate)
	if err != nil {
		return domain.VectorStore{}, err
	}
	if err := candidate.Validate(); err != nil {
		return domain.VectorStore{}, err
	}
	if err := validateConfig(entry, candidate.Config); err != nil {
		return domain.VectorStore{}, err
	}
	if err := entry.Validate(candidate); err != nil {
		return domain.VectorStore{}, err
	}
	return candidate, nil
}

// ImmutableChanged 比较索引及连接身份，秘密轮换本身不改变连接归属。
func (v *Vectors) ImmutableChanged(old, next domain.VectorStore) bool {
	if old.Provider != next.Provider {
		return true
	}
	entry, exists := v.entries[next.Provider]
	if !exists {
		return true
	}
	var err error
	old, err = normalize(entry, old)
	if err != nil {
		return true
	}
	next, err = normalize(entry, next)
	if err != nil {
		return true
	}
	for _, field := range entry.Definition.Fields {
		if field.Immutable && !field.Secret && old.Config[field.Key] != next.Config[field.Key] {
			return true
		}
	}
	return false
}

// ValidateStore 检查完整已保存配置，包括必填秘密及驱动语义。
func (v *Vectors) ValidateStore(store domain.VectorStore) error {
	_, err := v.PrepareStore(store)
	return err
}
func (v *Vectors) resolve(store domain.VectorStore) (port.Vectors, domain.VectorStore, error) {
	prepared, err := v.PrepareStore(store)
	if err != nil {
		return nil, domain.VectorStore{}, err
	}
	return v.entries[store.Provider].Driver, prepared, nil
}

// Test 检查已授权连接的实际服务。
func (v *Vectors) Test(ctx context.Context, store domain.VectorStore) error {
	driver, store, err := v.resolve(store)
	if err != nil {
		return err
	}
	return driver.Test(ctx, store)
}

// ReplaceDocument 保留企业、知识库、资料和处理代的写入范围。
func (v *Vectors) ReplaceDocument(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string, points []port.VectorPoint) error {
	driver, store, err := v.resolve(store)
	if err != nil {
		return err
	}
	return driver.ReplaceDocument(ctx, store, tenant, base, document, attempt, points)
}

// DeleteDocument 只删除指定资料的索引。
func (v *Vectors) DeleteDocument(ctx context.Context, store domain.VectorStore, tenant, base, document string) error {
	driver, store, err := v.resolve(store)
	if err != nil {
		return err
	}
	return driver.DeleteDocument(ctx, store, tenant, base, document)
}

// DeleteAttempt 不扩展调用方指定的失效处理代。
func (v *Vectors) DeleteAttempt(ctx context.Context, store domain.VectorStore, tenant, base, document, attempt string) error {
	driver, store, err := v.resolve(store)
	if err != nil {
		return err
	}
	return driver.DeleteAttempt(ctx, store, tenant, base, document, attempt)
}

// Search 返回索引命中，业务用例继续复核当前资料归属。
func (v *Vectors) Search(ctx context.Context, store domain.VectorStore, tenant, base string, query []float32, limit int) ([]port.VectorHit, error) {
	driver, store, err := v.resolve(store)
	if err != nil {
		return nil, err
	}
	return driver.Search(ctx, store, tenant, base, query, limit)
}

var _ port.Vectors = (*Vectors)(nil)
var _ port.VectorCatalog = (*Vectors)(nil)
