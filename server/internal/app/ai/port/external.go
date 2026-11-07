// Package port 定义模型配置实际需要的授权、服务探测及引用契约。
package port

import (
	"context"
	"go-server/internal/app/ai/dto"
	"go-server/internal/domain/ai"
)

type Actor struct{ TenantID, UserID, Role string }
type Authorizer interface {
	Authorize(context.Context, string, string) (Actor, error)
}
type Probe interface {
	Discover(context.Context, ai.Model) ([]dto.ModelOption, error)
	Test(context.Context, ai.Model) (int, error)
}

// Catalog 是已注册模型能力的只读入口，不读取企业凭据。
type Catalog interface {
	Providers() []dto.Provider
	ValidateModel(ai.Model) error
}

// PresetCatalog 提供与模型服务商独立的 Prompt 和文本切分预设。
type PresetCatalog interface {
	Presets() dto.Presets
}
type References interface {
	ModelReferences(context.Context, string, string) (int64, error)
	VectorStoreExists(context.Context, string, string) error
}
