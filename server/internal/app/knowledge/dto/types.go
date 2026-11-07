// Package dto 定义知识库页面与应用用例的输入和读取契约。
package dto

import "time"

// Identity 引用已存在的企业会话，不接受请求体提供的企业或用户 ID。
type Identity struct{ Hostname, Token string }

// OK 表示已完成请求。
type OK struct {
	OK bool `json:"ok"`
}

// VectorStoreInput 配置一个供应商连接。
type VectorStoreInput struct {
	Name     string            `json:"name"`
	Provider string            `json:"provider"`
	Config   map[string]string `json:"config"`
}

// VectorStore 提供已授权企业的完整连接配置。
type VectorStore struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Provider   string            `json:"provider"`
	Config     map[string]string `json:"config"`
	Referenced bool              `json:"referenced"`
	CreatedAt  time.Time         `json:"created_at"`
	UpdatedAt  time.Time         `json:"updated_at"`
}

// VectorFieldOption 是注册模块声明的受控选项。
type VectorFieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// VectorField 同时描述动态表单及基础校验；驱动继续负责协议语义校验。
type VectorField struct {
	Key         string              `json:"key"`
	Label       string              `json:"label"`
	Type        string              `json:"type"`
	Required    bool                `json:"required"`
	Secret      bool                `json:"secret"`
	Immutable   bool                `json:"immutable"`
	Default     string              `json:"default"`
	Placeholder string              `json:"placeholder"`
	Help        string              `json:"help"`
	Options     []VectorFieldOption `json:"options"`
}

// VectorProvider 描述已注册引擎的页面预设，不包含企业连接或凭据。
type VectorProvider struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Logo        string        `json:"logo"`
	Protocol    string        `json:"protocol"`
	Fields      []VectorField `json:"fields"`
}

// BaseInput 是知识库完整配置表单。
type BaseInput struct {
	Name             string `json:"name"`
	Description      string `json:"description"`
	IndexingMode     string `json:"indexing_mode"`
	RetrievalMode    string `json:"retrieval_mode"`
	ChunkStrategy    string `json:"chunk_strategy"`
	EmbeddingModelID string `json:"embedding_model_id"`
	VectorStoreID    string `json:"vector_store_id"`
	ParserEndpoint   string `json:"parser_endpoint"`
	ChunkSize        int    `json:"chunk_size"`
	ChunkOverlap     *int   `json:"chunk_overlap"`
}

// Base 汇总知识库配置和当前资料数量。
type Base struct {
	ID               string    `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	IndexingMode     string    `json:"indexing_mode"`
	RetrievalMode    string    `json:"retrieval_mode"`
	ChunkStrategy    string    `json:"chunk_strategy"`
	EmbeddingModelID string    `json:"embedding_model_id"`
	VectorStoreID    string    `json:"vector_store_id"`
	ParserEndpoint   string    `json:"parser_endpoint"`
	ChunkSize        int       `json:"chunk_size"`
	ChunkOverlap     int       `json:"chunk_overlap"`
	DocumentCount    int64     `json:"document_count"`
	ChunkCount       int64     `json:"chunk_count"`
	ProcessingCount  int64     `json:"processing_count"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// DocumentInput 创建或完整编辑手写资料。
type DocumentInput struct {
	Title   string `json:"title"`
	Content string `json:"content"`
}

// ChunkInput 编辑派生分块，不覆盖原始文件。
type ChunkInput struct {
	Content string `json:"content"`
}

// Document 保存处理状态；Content 仅在详情接口返回。
type Document struct {
	ID           string    `json:"id"`
	BaseID       string    `json:"base_id"`
	Title        string    `json:"title"`
	Source       string    `json:"source"`
	FileName     string    `json:"file_name"`
	FileType     string    `json:"file_type"`
	FileSize     int64     `json:"file_size"`
	ParseStatus  string    `json:"parse_status"`
	ErrorMessage string    `json:"error_message"`
	ChunkCount   int64     `json:"chunk_count"`
	Content      string    `json:"content,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Chunk 展示分块及在解析原文中的字符位置。
type Chunk struct {
	ID            string `json:"id"`
	DocumentID    string `json:"document_id"`
	Content       string `json:"content"`
	ContextHeader string `json:"context_header"`
	Index         int    `json:"index"`
	Start         int    `json:"start"`
	End           int    `json:"end"`
}

// SearchInput 指定检索文本和数量；Mode 缺省使用知识库默认模式。
type SearchInput struct {
	Query string `json:"query"`
	Mode  string `json:"mode"`
	Limit int    `json:"limit"`
}

// Hit 返回已确认当前版本的检索分块。
type Hit struct {
	Chunk
	DocumentTitle string  `json:"document_title"`
	FileName      string  `json:"file_name"`
	Score         float64 `json:"score"`
}

// PreviewInput 仅预览切分结果，不创建资料或索引。
type PreviewInput struct {
	Content       string `json:"content"`
	ChunkStrategy string `json:"chunk_strategy"`
	ChunkSize     int    `json:"chunk_size"`
	ChunkOverlap  int    `json:"chunk_overlap"`
}

// PreviewChunk 返回原文范围及独立标题上下文。
type PreviewChunk struct {
	Content       string `json:"content"`
	ContextHeader string `json:"context_header"`
	Index         int    `json:"index"`
	Start         int    `json:"start"`
	End           int    `json:"end"`
}
