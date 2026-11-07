// Package port 定义知识库用例所需的身份、解析、向量化和索引契约。
package port

import (
	"context"
	"go-server/internal/app/knowledge/dto"
	"go-server/internal/domain/ai"
	"go-server/internal/domain/knowledge"
)

// Actor 来自已有 SaaS 会话验证，不能由知识库请求体指定。
type Actor struct{ TenantID, UserID, Role string }

// Authorizer 验证请求域名、企业及当前会话。
type Authorizer interface {
	Authorize(context.Context, string, string) (Actor, error)
}

// ChunkDraft 由解析切分能力产生；Start 和 End 使用 Unicode 字符位置。
type ChunkDraft struct {
	Content           string
	ContextHeader     string
	Index, Start, End int
}

// EmbeddingContent 保持解析正文偏移，单独组合标题上下文。
func (c ChunkDraft) EmbeddingContent() string {
	return (knowledge.Chunk{Content: c.Content, ContextHeader: c.ContextHeader}).EmbeddingContent()
}

// Models 只消费已授权企业的模型和新建默认模板，配置写入由 AI 用例负责。
type Models interface {
	ResolveModel(context.Context, string, string, string) (ai.Model, error)
	Defaults(context.Context, string) (ai.Settings, error)
}

// Processor 将文件统一为文本并使用同一切分规则产生分块。
type Processor interface {
	Parse(context.Context, string, []byte, string) (string, error)
	Split(string, knowledge.ChunkingConfig) []ChunkDraft
}

// Embedder 使用具体页面配置的模型生成文本向量。
type Embedder interface {
	Embed(context.Context, ai.Model, []string) ([][]float32, error)
}

// VectorCatalog 统一提供实际注册引擎的预设和保存校验。
type VectorCatalog interface {
	Providers() []dto.VectorProvider
	ValidateStore(knowledge.VectorStore) error
	ValidateInput(knowledge.VectorStore) error
	PrepareStore(knowledge.VectorStore) (knowledge.VectorStore, error)
	ImmutableChanged(knowledge.VectorStore, knowledge.VectorStore) bool
}

// VectorPoint 是派生索引项；ID 对应业务分块，原文仍由业务库维护。
type VectorPoint struct {
	ID, Content string
	Vector      []float32
}

// VectorHit 的 ID 必须再经过业务库归属和处理代验证，不能直接作为公开内容。
type VectorHit struct {
	ID    string
	Score float64
}

// Vectors 以企业、知识库和资料共同限定索引写入与删除范围。
// ReplaceDocument 只提交给定 attempt 的点，不能删除其他处理代的新点。
type Vectors interface {
	Test(context.Context, knowledge.VectorStore) error
	ReplaceDocument(context.Context, knowledge.VectorStore, string, string, string, string, []VectorPoint) error
	DeleteDocument(context.Context, knowledge.VectorStore, string, string, string) error
	DeleteAttempt(context.Context, knowledge.VectorStore, string, string, string, string) error
	Search(context.Context, knowledge.VectorStore, string, string, []float32, int) ([]VectorHit, error)
}
