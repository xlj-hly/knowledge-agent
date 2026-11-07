package ai

import (
	"context"
	"errors"
	"net/http"
	"sort"

	"go-server/internal/app/ai/dto"
	domain "go-server/internal/domain/ai"
	"go-server/internal/infra/ai/providers"
	"go-server/internal/infra/httpclient"
)

func option(id, kind string, dimension int) dto.ModelOption {
	return dto.ModelOption{ID: id, Name: id, Kind: kind, Dimension: dimension}
}

func (p *protocolDriver) Discover(ctx context.Context, definition providers.Definition, m domain.Model) ([]dto.ModelOption, error) {
	suffix := definition.Compatibility.DiscoveryPath
	if m.Kind == "embedding" && definition.Compatibility.EmbeddingDiscoveryPath != "" {
		suffix = definition.Compatibility.EmbeddingDiscoveryPath
	}
	if suffix == "" {
		return nil, errors.New("该服务商不支持远端模型发现，请手动填写模型名")
	}
	address, err := protocolEndpoint(m.Endpoint, definition.Compatibility.RootPath, suffix)
	if err != nil {
		return nil, err
	}
	var response struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
		Error *struct{} `json:"error"`
	}
	if _, err = httpclient.JSON(ctx, p.client, http.MethodGet, address, headers(m), nil, &response, 2<<20); err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, errors.New("模型列表服务返回错误")
	}
	rows := make([]dto.ModelOption, 0)
	seen := map[string]bool{}
	add := func(id string) {
		if id == "" || len(id) > 256 || seen[id] {
			return
		}
		seen[id] = true
		d := 0
		for _, preset := range definition.Models {
			if preset.ID == id && preset.Kind == m.Kind {
				d = preset.Dimension
			}
		}
		rows = append(rows, option(id, m.Kind, d))
	}
	if p.protocol == "ollama" {
		for _, r := range response.Models {
			add(r.Name)
		}
	} else {
		for _, r := range response.Data {
			add(r.ID)
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}
func (p *protocolDriver) Test(ctx context.Context, definition providers.Definition, m domain.Model) (int, error) {
	if m.Kind == "embedding" {
		vectors, err := p.EmbedBatch(ctx, definition, m, []string{"模型连接测试"})
		if err != nil {
			return 0, err
		}
		if len(vectors) != 1 {
			return 0, errors.New("模型未返回测试向量")
		}
		dimension := len(vectors[0])
		if dimension < 1 || dimension > 65536 {
			return 0, errors.New("模型向量维度无效")
		}
		if err = validateVectors(vectors, 1, dimension); err != nil {
			return 0, err
		}
		return dimension, nil
	}
	suffix := "/chat/completions"
	if p.protocol == "ollama" {
		suffix = "/api/chat"
	}
	address, err := protocolEndpoint(m.Endpoint, definition.Compatibility.RootPath, suffix)
	if err != nil {
		return 0, err
	}
	request := map[string]any{"model": m.ModelName, "messages": []map[string]string{{"role": "user", "content": "请回复：连接成功"}}, "stream": false}
	if p.protocol == "ollama" {
		request["options"] = map[string]int{"num_predict": 16}
	} else {
		request[definition.Compatibility.TokenField] = 16
	}
	var response struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content"`
			} `json:"message"`
		} `json:"choices"`
		Message struct {
			Content  string `json:"content"`
			Thinking string `json:"thinking"`
		} `json:"message"`
		Error *struct{} `json:"error"`
	}
	if _, err = httpclient.JSON(ctx, p.client, http.MethodPost, address, headers(m), request, &response, 1<<20); err != nil {
		return 0, err
	}
	if response.Error != nil {
		return 0, errors.New("模型返回推理错误")
	}
	if p.protocol == "ollama" {
		if response.Message.Content == "" && response.Message.Thinking == "" {
			return 0, errors.New("模型没有返回有效回答")
		}
	} else if len(response.Choices) == 0 || (response.Choices[0].Message.Content == "" && response.Choices[0].Message.ReasoningContent == "") {
		return 0, errors.New("模型没有返回有效回答")
	}
	return 0, nil
}
