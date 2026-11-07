package ai

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	domain "go-server/internal/domain/ai"
	"go-server/internal/infra/ai/providers"
	"go-server/internal/infra/httpclient"
)

func (e *protocolDriver) EmbedBatch(ctx context.Context, definition providers.Definition, model domain.Model, texts []string) ([][]float32, error) {
	suffix := "/embeddings"
	if e.protocol == "ollama" {
		suffix = "/api/embed"
	}
	address, err := protocolEndpoint(model.Endpoint, definition.Compatibility.RootPath, suffix)
	if err != nil {
		return nil, err
	}

	headers := make(map[string]string)
	if model.APIKey != "" {
		headers["Authorization"] = "Bearer " + model.APIKey
	}
	input := map[string]any{"model": model.ModelName, "input": texts}
	if e.protocol == "ollama" {
		input["truncate"] = false
		var output struct {
			Embeddings [][]float32 `json:"embeddings"`
		}
		if _, err := httpclient.JSON(ctx, e.client, http.MethodPost, address, headers, input, &output, 16<<20); err != nil {
			return nil, fmt.Errorf("Ollama 向量化失败: %w", err)
		}
		return output.Embeddings, nil
	}
	var output struct {
		Data []struct {
			Index     *int      `json:"index"`
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
		Error *struct{} `json:"error"`
	}
	if _, err := httpclient.JSON(ctx, e.client, http.MethodPost, address, headers, input, &output, 16<<20); err != nil {
		return nil, fmt.Errorf("Embedding 向量化失败: %w", err)
	}
	if output.Error != nil {
		return nil, errors.New("Embedding 服务返回模型错误")
	}
	if len(output.Data) != len(texts) {
		return nil, errors.New("Embedding 返回的向量数量与输入不一致")
	}
	vectors := make([][]float32, len(texts))
	seen := make([]bool, len(texts))
	for position, item := range output.Data {
		index := position
		if item.Index != nil {
			index = *item.Index
		}
		if index < 0 || index >= len(texts) || seen[index] {
			return nil, errors.New("Embedding 返回的向量索引无效或重复")
		}
		seen[index] = true
		vectors[index] = item.Embedding
	}
	return vectors, nil
}
