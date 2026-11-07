package ai

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	domain "go-server/internal/domain/ai"
)

// Embedder 按注册目录解析模型协议，负责有界批次和向量不变量。
type Embedder struct{ registry *Registry }

// NewEmbedder 使用组装阶段创建的只读能力目录。
func NewEmbedder(registry *Registry) *Embedder { return &Embedder{registry: registry} }

// Embed 保持输入顺序并验证维度，外部故障不会退化为替代向量。
func (e *Embedder) Embed(ctx context.Context, model domain.Model, texts []string) ([][]float32, error) {
	if model.Kind != "embedding" || model.Dimension < 1 || model.Dimension > 65536 || strings.TrimSpace(model.ModelName) == "" {
		return nil, errors.New("Embedding 模型名称、用途或维度无效")
	}
	definition, driver, err := e.registry.resolve(model.Provider, "embedding")
	if err != nil {
		return nil, err
	}
	result := make([][]float32, 0, len(texts))
	// 有界批次减少响应体与供应商批次限制的影响，不自动重试计费调用。
	for start := 0; start < len(texts); start += 32 {
		batch := texts[start:min(start+32, len(texts))]
		vectors, err := driver.EmbedBatch(ctx, definition, model, batch)
		if err != nil {
			return nil, err
		}
		if err := validateVectors(vectors, len(batch), model.Dimension); err != nil {
			return nil, err
		}
		result = append(result, vectors...)
	}
	return result, nil
}

func validateVectors(vectors [][]float32, count, dimension int) error {
	if len(vectors) != count {
		return errors.New("Embedding 返回的向量数量与输入不一致")
	}
	for _, vector := range vectors {
		if len(vector) != dimension {
			return fmt.Errorf("Embedding 返回维度与配置不一致，预期 %d，实际 %d", dimension, len(vector))
		}
		var magnitude float64
		for _, value := range vector {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return errors.New("Embedding 返回了无效数值")
			}
			magnitude += float64(value) * float64(value)
		}
		if magnitude == 0 {
			return errors.New("Embedding 返回了零向量")
		}
	}
	return nil
}
