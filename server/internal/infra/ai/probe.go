package ai

import (
	"context"

	"go-server/internal/app/ai/dto"
	domain "go-server/internal/domain/ai"
)

// Probe 仅执行模型发现和调用检测，目录由 Registry 独立提供。
type Probe struct{ registry *Registry }

// NewProbe 使用与实际生成和向量化共享的能力目录。
func NewProbe(registry *Registry) *Probe { return &Probe{registry: registry} }

// Discover 调用已注册的服务商发现协议。
func (p *Probe) Discover(ctx context.Context, model domain.Model) ([]dto.ModelOption, error) {
	definition, driver, err := p.registry.resolve(model.Provider, model.Kind)
	if err != nil {
		return nil, err
	}
	return driver.Discover(ctx, definition, model)
}

// Test 检测当前配置；检测向量维度不要求预先知道输出维度。
func (p *Probe) Test(ctx context.Context, model domain.Model) (int, error) {
	definition, driver, err := p.registry.resolve(model.Provider, model.Kind)
	if err != nil {
		return 0, err
	}
	return driver.Test(ctx, definition, model)
}
