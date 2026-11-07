package knowledge

import (
	"context"
	"encoding/base64"
	"errors"
	"go-server/internal/infra/httpclient"
	"net/http"
	"path/filepath"
	"strings"

	"go-server/internal/app/knowledge/port"
	domain "go-server/internal/domain/knowledge"
	"go-server/internal/infra/knowledge/chunker"
	"go-server/internal/infra/knowledge/parser"
)

// Processor 复用 WeKnora 文本切分和转换，PDF 使用其 DocReader HTTP 协议。
type Processor struct{ client *http.Client }

// NewProcessor 创建无隐式环境依赖的文档处理适配器。
func NewProcessor() *Processor { return &Processor{client: httpclient.New()} }

// Parse 返回 UTF-8 正文；PDF 必须配置可用的 DocReader 服务。
func (p *Processor) Parse(ctx context.Context, filename string, data []byte, parserEndpoint string) (string, error) {
	if strings.ToLower(filepath.Ext(filename)) != ".pdf" {
		return parser.Parse(ctx, filename, data)
	}
	if len(data) > 10<<20 {
		return "", errors.New("资料大小不能超过 10 MiB")
	}
	if strings.TrimSpace(parserEndpoint) == "" {
		return "", errors.New("PDF 解析需要在知识库设置中配置 DocReader 地址")
	}
	address, err := httpclient.Endpoint(parserEndpoint, "/read")
	if err != nil {
		return "", err
	}
	input := struct {
		Content string `json:"file_content"`
		Name    string `json:"file_name"`
		Type    string `json:"file_type"`
	}{base64.StdEncoding.EncodeToString(data), filepath.Base(filename), "pdf"}
	var output struct {
		Content string `json:"markdown_content"`
		Error   string `json:"error"`
	}
	if _, err := httpclient.JSON(ctx, p.client, http.MethodPost, address, nil, input, &output, 4<<20); err != nil {
		return "", err
	}
	if output.Error != "" {
		return "", errors.New("DocReader 未能解析该 PDF，请检查服务日志")
	}
	if len(output.Content) > 2<<20 {
		return "", errors.New("解析后的正文不能超过 2 MiB")
	}
	if strings.ContainsRune(output.Content, 0) {
		return "", errors.New("解析后的正文包含无效空字符")
	}
	return output.Content, nil
}

// Split 保留正文字符偏移，统一使用 WeKnora 的自适应切分算法。
func (p *Processor) Split(text string, cfg domain.ChunkingConfig) []port.ChunkDraft {
	chunks := chunker.Split(text, chunker.SplitterConfig{ChunkSize: cfg.Size, ChunkOverlap: cfg.Overlap, Strategy: cfg.Strategy})
	result := make([]port.ChunkDraft, 0, len(chunks))
	for _, chunk := range chunks {
		if strings.TrimSpace(chunk.Content) == "" {
			continue
		}
		result = append(result, port.ChunkDraft{Content: chunk.Content, ContextHeader: chunk.ContextHeader, Index: len(result), Start: chunk.Start, End: chunk.End})
	}
	return result
}
