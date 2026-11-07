// Package presets 维护独立于模型服务商的 Prompt 与文本切分预设目录。
package presets

import (
	"fmt"
	"strings"

	"go-server/internal/app/ai/dto"
	domain "go-server/internal/domain/ai"
	saas "go-server/internal/domain/saas"
)

// Catalog 在构造后只读；企业实际配置仍由业务仓储持久化。
type Catalog struct {
	definitions dto.Presets
}

// NewBuiltin 显式组装产品自带的 Prompt、分块模板和已有切分算法说明。
func NewBuiltin() (*Catalog, error) {
	return New(dto.Presets{
		Prompts:         promptDefinitions(),
		Chunking:        chunkDefinitions(),
		ChunkStrategies: strategyDefinitions(),
	})
}

// New 校验并复制预设；ID 在各自类别内唯一，切分模板只能引用已声明的现有算法。
func New(definitions dto.Presets) (*Catalog, error) {
	strategies := make(map[string]struct{}, len(definitions.ChunkStrategies))
	for _, strategy := range definitions.ChunkStrategies {
		if err := register(strategies, strategy.ID, strategy.Name); err != nil {
			return nil, fmt.Errorf("切分策略预设: %w", err)
		}
		if strings.TrimSpace(strategy.Description) == "" || len(strategy.Description) > 1000 || strings.ContainsRune(strategy.Description, 0) {
			return nil, fmt.Errorf("切分策略 %q 的说明无效", strategy.ID)
		}
		settings := domain.DefaultSettings()
		settings.ChunkStrategy = strategy.ID
		if err := settings.Validate(); err != nil {
			return nil, fmt.Errorf("切分策略 %q: %w", strategy.ID, err)
		}
	}
	prompts := make(map[string]struct{}, len(definitions.Prompts))
	for _, prompt := range definitions.Prompts {
		if err := register(prompts, prompt.ID, prompt.Name); err != nil {
			return nil, fmt.Errorf("Prompt 预设: %w", err)
		}
		if strings.TrimSpace(prompt.Content) == "" {
			return nil, fmt.Errorf("Prompt 预设 %q 的内容为空", prompt.ID)
		}
		settings := domain.DefaultSettings()
		settings.SystemPrompt = prompt.Content
		if err := settings.Validate(); err != nil {
			return nil, fmt.Errorf("Prompt 预设 %q: %w", prompt.ID, err)
		}
	}
	chunks := make(map[string]struct{}, len(definitions.Chunking))
	for _, chunk := range definitions.Chunking {
		if err := register(chunks, chunk.ID, chunk.Name); err != nil {
			return nil, fmt.Errorf("分块预设: %w", err)
		}
		if _, ok := strategies[chunk.Strategy]; !ok {
			return nil, fmt.Errorf("分块预设 %q 引用了未声明的切分策略 %q", chunk.ID, chunk.Strategy)
		}
		settings := domain.DefaultSettings()
		settings.ChunkStrategy, settings.ChunkSize, settings.ChunkOverlap = chunk.Strategy, chunk.Size, chunk.Overlap
		if err := settings.Validate(); err != nil {
			return nil, fmt.Errorf("分块预设 %q: %w", chunk.ID, err)
		}
	}
	return &Catalog{definitions: clone(definitions)}, nil
}

// Presets 返回独立副本，调用方不能改变目录或其他请求看到的预设。
func (c *Catalog) Presets() dto.Presets {
	return clone(c.definitions)
}

func clone(definitions dto.Presets) dto.Presets {
	result := dto.Presets{
		Prompts:         make([]dto.PromptPreset, len(definitions.Prompts)),
		Chunking:        make([]dto.ChunkPreset, len(definitions.Chunking)),
		ChunkStrategies: make([]dto.ChunkStrategy, len(definitions.ChunkStrategies)),
	}
	copy(result.Prompts, definitions.Prompts)
	copy(result.Chunking, definitions.Chunking)
	copy(result.ChunkStrategies, definitions.ChunkStrategies)
	return result
}

func register(ids map[string]struct{}, id, name string) error {
	if id == "" || len(id) > 64 {
		return fmt.Errorf("预设标识长度须为 1–64 字节")
	}
	for _, char := range id {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' && char != '_' {
			return fmt.Errorf("预设标识 %q 须使用小写字母、数字、连字符或下划线", id)
		}
	}
	if err := saas.ValidateName(name, 128, "预设名称"); err != nil {
		return err
	}
	if strings.ContainsRune(name, 0) {
		return fmt.Errorf("预设名称不能包含 NUL")
	}
	if _, exists := ids[id]; exists {
		return fmt.Errorf("重复预设标识 %q", id)
	}
	ids[id] = struct{}{}
	return nil
}
