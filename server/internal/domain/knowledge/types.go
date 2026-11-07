// Package knowledge 定义知识库、资料、分块及其配置的领域对象。
package knowledge

import (
	"strings"
	"time"
)

// VectorStore 保存具体引擎的配置；配置键和语义由注册模块校验。
type VectorStore struct {
	ID, TenantID, Name, Provider string
	Config                       map[string]string
	CreatedAt, UpdatedAt         time.Time
}

// Base 将资料集合和既定处理配置关联；已有资料时不得直接修改索引空间。
type Base struct {
	ID, TenantID, CreatorID, Name, Description                                                  string
	EmbeddingModelID, VectorStoreID, IndexingMode, RetrievalMode, ParserEndpoint, ChunkStrategy string
	ChunkSize, ChunkOverlap                                                                     int
	DocumentCount, ChunkCount, ProcessingCount                                                  int64
	CreatedAt, UpdatedAt                                                                        time.Time
}

// Document 保存原资料和处理状态；Attempt 标识当前处理代，旧任务不能覆盖新结果。
type Document struct {
	ID, TenantID, BaseID, Title, Source, FileName, FileType, ObjectKey, FileHash string
	Content, ParseStatus, ErrorMessage, Attempt                                  string
	// IndexStoreID 是当前处理代的存储快照；换库重建后仍能回收原连接里的旧向量。
	IndexStoreID         string
	CleanupTargets       []CleanupTarget
	UseChunks            bool
	FileSize             int64
	ChunkCount           int64
	LeaseUntil           *time.Time
	CreatedAt, UpdatedAt time.Time
}

// Chunk 保留与原文的 Unicode 字符位置，Attempt 将内容与相应索引版本绑定。
type Chunk struct {
	ID, TenantID, BaseID, DocumentID, Attempt, Content, ContextHeader string
	Index, Start, End                                                 int
	CreatedAt                                                         time.Time
}

// CleanupTarget 精确标识待回收的旧处理代，不能用新知识库配置替代其连接归属。
type CleanupTarget struct{ StoreID, Attempt string }

// RememberIndex 将当前处理代纳入幂等清理清单。
func (d *Document) RememberIndex() {
	if d.IndexStoreID == "" || d.Attempt == "" {
		return
	}
	target := CleanupTarget{StoreID: d.IndexStoreID, Attempt: d.Attempt}
	for _, old := range d.CleanupTargets {
		if old == target {
			return
		}
	}
	d.CleanupTargets = append(d.CleanupTargets, target)
}

// ChunkingConfig 是持久化到知识库的切分策略；尺寸使用 Unicode 字符数。
type ChunkingConfig struct {
	Strategy      string
	Size, Overlap int
}

// Chunking 返回知识库实际采用的切分配置。
func (b Base) Chunking() ChunkingConfig {
	return ChunkingConfig{Strategy: b.ChunkStrategy, Size: b.ChunkSize, Overlap: b.ChunkOverlap}
}

// EmbeddingContent 保留源文范围，同时把标题上下文传给检索与问答。
func (c Chunk) EmbeddingContent() string {
	body := strings.TrimSpace(c.Content)
	if c.ContextHeader == "" {
		return body
	}
	return c.ContextHeader + "\n\n" + body
}

// Hit 是已确认归属和当前索引代的检索结果。
type Hit struct {
	Chunk                   Chunk
	DocumentTitle, FileName string
	Score                   float64
}
